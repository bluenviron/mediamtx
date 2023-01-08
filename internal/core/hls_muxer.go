package core

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/codecs/mpeg4audio"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/media"
	"github.com/aler9/gortsplib/v2/pkg/ringbuffer"
	"github.com/gin-gonic/gin"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/formatprocessor"
	"github.com/aler9/rtsp-simple-server/internal/hls"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

const (
	closeCheckPeriod      = 1 * time.Second
	closeAfterInactivity  = 60 * time.Second
	hlsMuxerRecreatePause = 10 * time.Second
)

//go:embed hls_index.html
var hlsIndex []byte

type hlsMuxerResponse struct {
	muxer *hlsMuxer
	cb    func() *hls.MuxerFileResponse
}

type hlsMuxerRequest struct {
	dir  string
	file string
	ctx  *gin.Context
	res  chan hlsMuxerResponse
}

type hlsMuxerPathManager interface {
	readerAdd(req pathReaderAddReq) pathReaderSetupPlayRes
}

type hlsMuxerParent interface {
	log(logger.Level, string, ...interface{})
	muxerClose(*hlsMuxer)
}

type hlsMuxer struct {
	name                      string
	remoteAddr                string
	externalAuthenticationURL string
	alwaysRemux               bool
	variant                   conf.HLSVariant
	segmentCount              int
	segmentDuration           conf.StringDuration
	partDuration              conf.StringDuration
	segmentMaxSize            conf.StringSize
	readBufferCount           int
	wg                        *sync.WaitGroup
	pathName                  string
	pathManager               hlsMuxerPathManager
	parent                    hlsMuxerParent

	ctx             context.Context
	ctxCancel       func()
	created         time.Time
	path            *path
	ringBuffer      *ringbuffer.RingBuffer
	lastRequestTime *int64
	muxer           *hls.Muxer
	requests        []*hlsMuxerRequest
	bytesSent       *uint64

	// in
	chRequest          chan *hlsMuxerRequest
	chAPIHLSMuxersList chan hlsServerAPIMuxersListSubReq
}

func newHLSMuxer(
	parentCtx context.Context,
	name string,
	remoteAddr string,
	externalAuthenticationURL string,
	alwaysRemux bool,
	variant conf.HLSVariant,
	segmentCount int,
	segmentDuration conf.StringDuration,
	partDuration conf.StringDuration,
	segmentMaxSize conf.StringSize,
	readBufferCount int,
	req *hlsMuxerRequest,
	wg *sync.WaitGroup,
	pathName string,
	pathManager hlsMuxerPathManager,
	parent hlsMuxerParent,
) *hlsMuxer {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	m := &hlsMuxer{
		name:                      name,
		remoteAddr:                remoteAddr,
		externalAuthenticationURL: externalAuthenticationURL,
		alwaysRemux:               alwaysRemux,
		variant:                   variant,
		segmentCount:              segmentCount,
		segmentDuration:           segmentDuration,
		partDuration:              partDuration,
		segmentMaxSize:            segmentMaxSize,
		readBufferCount:           readBufferCount,
		wg:                        wg,
		pathName:                  pathName,
		pathManager:               pathManager,
		parent:                    parent,
		ctx:                       ctx,
		ctxCancel:                 ctxCancel,
		created:                   time.Now(),
		lastRequestTime: func() *int64 {
			v := time.Now().UnixNano()
			return &v
		}(),
		bytesSent:          new(uint64),
		chRequest:          make(chan *hlsMuxerRequest),
		chAPIHLSMuxersList: make(chan hlsServerAPIMuxersListSubReq),
	}

	if req != nil {
		m.requests = append(m.requests, req)
	}

	m.log(logger.Info, "created %s", func() string {
		if remoteAddr == "" {
			return "automatically"
		}
		return "(requested by " + remoteAddr + ")"
	}())

	m.wg.Add(1)
	go m.run()

	return m
}

func (m *hlsMuxer) close() {
	m.ctxCancel()
}

func (m *hlsMuxer) log(level logger.Level, format string, args ...interface{}) {
	m.parent.log(level, "[muxer %s] "+format, append([]interface{}{m.pathName}, args...)...)
}

// PathName returns the path name.
func (m *hlsMuxer) PathName() string {
	return m.pathName
}

func (m *hlsMuxer) run() {
	defer m.wg.Done()

	err := func() error {
		var innerReady chan struct{}
		var innerErr chan error
		var innerCtx context.Context
		var innerCtxCancel func()

		createInner := func() {
			innerReady = make(chan struct{})
			innerErr = make(chan error)
			innerCtx, innerCtxCancel = context.WithCancel(context.Background())
			go func() {
				innerErr <- m.runInner(innerCtx, innerReady)
			}()
		}

		createInner()

		isReady := false
		isRecreating := false
		recreateTimer := newEmptyTimer()

		for {
			select {
			case <-m.ctx.Done():
				if !isRecreating {
					innerCtxCancel()
					<-innerErr
				}
				return errors.New("terminated")

			case req := <-m.chRequest:
				if isReady {
					req.res <- hlsMuxerResponse{
						muxer: m,
						cb:    m.handleRequest(req),
					}
				} else {
					m.requests = append(m.requests, req)
				}

			case req := <-m.chAPIHLSMuxersList:
				req.data.Items[m.name] = hlsServerAPIMuxersListItem{
					Created:     m.created,
					LastRequest: time.Unix(0, atomic.LoadInt64(m.lastRequestTime)),
					BytesSent:   atomic.LoadUint64(m.bytesSent),
				}
				close(req.res)

			case <-innerReady:
				isReady = true
				for _, req := range m.requests {
					req.res <- hlsMuxerResponse{
						muxer: m,
						cb:    m.handleRequest(req),
					}
				}
				m.requests = nil

			case err := <-innerErr:
				innerCtxCancel()

				if m.alwaysRemux {
					m.log(logger.Info, "ERR: %v", err)
					m.clearQueuedRequests()
					isReady = false
					isRecreating = true
					recreateTimer = time.NewTimer(hlsMuxerRecreatePause)
				} else {
					return err
				}

			case <-recreateTimer.C:
				isRecreating = false
				createInner()
			}
		}
	}()

	m.ctxCancel()

	m.clearQueuedRequests()

	m.parent.muxerClose(m)

	m.log(logger.Info, "destroyed (%v)", err)
}

func (m *hlsMuxer) clearQueuedRequests() {
	for _, req := range m.requests {
		req.res <- hlsMuxerResponse{
			muxer: m,
			cb: func() *hls.MuxerFileResponse {
				return &hls.MuxerFileResponse{Status: http.StatusNotFound}
			},
		}
	}
}

func (m *hlsMuxer) runInner(innerCtx context.Context, innerReady chan struct{}) error {
	res := m.pathManager.readerAdd(pathReaderAddReq{
		author:   m,
		pathName: m.pathName,
	})
	if res.err != nil {
		return res.err
	}

	m.path = res.path

	defer func() {
		m.path.readerRemove(pathReaderRemoveReq{author: m})
	}()

	m.ringBuffer, _ = ringbuffer.New(uint64(m.readBufferCount))

	var medias media.Medias

	videoMedia, videoFormat := m.setupVideoMedia(res.stream)
	if videoMedia != nil {
		medias = append(medias, videoMedia)
	}

	audioMedia, audioFormat := m.setupAudioMedia(res.stream)
	if audioMedia != nil {
		medias = append(medias, audioMedia)
	}

	defer res.stream.readerRemove(m)

	if medias == nil {
		return fmt.Errorf(
			"the stream doesn't contain any supported codec (which are currently H264, H265, MPEG4-Audio, Opus)")
	}

	var err error
	m.muxer, err = hls.NewMuxer(
		hls.MuxerVariant(m.variant),
		m.segmentCount,
		time.Duration(m.segmentDuration),
		time.Duration(m.partDuration),
		uint64(m.segmentMaxSize),
		videoFormat,
		audioFormat,
	)
	if err != nil {
		return fmt.Errorf("muxer error: %v", err)
	}
	defer m.muxer.Close()

	innerReady <- struct{}{}

	m.log(logger.Info, "is converting into HLS, %s",
		sourceMediaInfo(medias))

	writerDone := make(chan error)
	go func() {
		writerDone <- m.runWriter()
	}()

	closeCheckTicker := time.NewTicker(closeCheckPeriod)
	defer closeCheckTicker.Stop()

	for {
		select {
		case <-closeCheckTicker.C:
			if m.remoteAddr != "" {
				t := time.Unix(0, atomic.LoadInt64(m.lastRequestTime))
				if time.Since(t) >= closeAfterInactivity {
					m.ringBuffer.Close()
					<-writerDone
					return fmt.Errorf("not used anymore")
				}
			}

		case err := <-writerDone:
			return err

		case <-innerCtx.Done():
			m.ringBuffer.Close()
			<-writerDone
			return fmt.Errorf("terminated")
		}
	}
}

func (m *hlsMuxer) setupVideoMedia(stream *stream) (*media.Media, format.Format) {
	var videoFormatH265 *format.H265
	videoMedia := stream.medias().FindFormat(&videoFormatH265)

	if videoFormatH265 != nil {
		videoStartPTSFilled := false
		var videoStartPTS time.Duration

		stream.readerAdd(m, videoMedia, videoFormatH265, func(dat formatprocessor.Data) {
			m.ringBuffer.Push(func() error {
				tdata := dat.(*formatprocessor.DataH265)

				if tdata.AU == nil {
					return nil
				}

				if !videoStartPTSFilled {
					videoStartPTSFilled = true
					videoStartPTS = tdata.PTS
				}
				pts := tdata.PTS - videoStartPTS

				err := m.muxer.WriteH26x(tdata.NTP, pts, tdata.AU)
				if err != nil {
					return fmt.Errorf("muxer error: %v", err)
				}

				return nil
			})
		})

		return videoMedia, videoFormatH265
	}

	var videoFormatH264 *format.H264
	videoMedia = stream.medias().FindFormat(&videoFormatH264)

	if videoFormatH264 != nil {
		videoStartPTSFilled := false
		var videoStartPTS time.Duration

		stream.readerAdd(m, videoMedia, videoFormatH264, func(dat formatprocessor.Data) {
			m.ringBuffer.Push(func() error {
				tdata := dat.(*formatprocessor.DataH264)

				if tdata.AU == nil {
					return nil
				}

				if !videoStartPTSFilled {
					videoStartPTSFilled = true
					videoStartPTS = tdata.PTS
				}
				pts := tdata.PTS - videoStartPTS

				err := m.muxer.WriteH26x(tdata.NTP, pts, tdata.AU)
				if err != nil {
					return fmt.Errorf("muxer error: %v", err)
				}

				return nil
			})
		})

		return videoMedia, videoFormatH264
	}

	return nil, nil
}

func (m *hlsMuxer) setupAudioMedia(stream *stream) (*media.Media, format.Format) {
	var audioFormatMPEG4Audio *format.MPEG4Audio
	audioMedia := stream.medias().FindFormat(&audioFormatMPEG4Audio)

	if audioFormatMPEG4Audio != nil {
		audioStartPTSFilled := false
		var audioStartPTS time.Duration

		stream.readerAdd(m, audioMedia, audioFormatMPEG4Audio, func(dat formatprocessor.Data) {
			m.ringBuffer.Push(func() error {
				tdata := dat.(*formatprocessor.DataMPEG4Audio)

				if tdata.AUs == nil {
					return nil
				}

				if !audioStartPTSFilled {
					audioStartPTSFilled = true
					audioStartPTS = tdata.PTS
				}
				pts := tdata.PTS - audioStartPTS

				for i, au := range tdata.AUs {
					err := m.muxer.WriteAudio(
						tdata.NTP,
						pts+time.Duration(i)*mpeg4audio.SamplesPerAccessUnit*
							time.Second/time.Duration(audioFormatMPEG4Audio.ClockRate()),
						au)
					if err != nil {
						return fmt.Errorf("muxer error: %v", err)
					}
				}

				return nil
			})
		})

		return audioMedia, audioFormatMPEG4Audio
	}

	var audioFormatOpus *format.Opus
	audioMedia = stream.medias().FindFormat(&audioFormatOpus)

	if audioFormatOpus != nil {
		audioStartPTSFilled := false
		var audioStartPTS time.Duration

		stream.readerAdd(m, audioMedia, audioFormatOpus, func(dat formatprocessor.Data) {
			m.ringBuffer.Push(func() error {
				tdata := dat.(*formatprocessor.DataOpus)

				if !audioStartPTSFilled {
					audioStartPTSFilled = true
					audioStartPTS = tdata.PTS
				}
				pts := tdata.PTS - audioStartPTS

				err := m.muxer.WriteAudio(
					tdata.NTP,
					pts,
					tdata.Frame)
				if err != nil {
					return fmt.Errorf("muxer error: %v", err)
				}

				return nil
			})
		})

		return audioMedia, audioFormatOpus
	}

	return nil, nil
}

func (m *hlsMuxer) runWriter() error {
	for {
		item, ok := m.ringBuffer.Pull()
		if !ok {
			return fmt.Errorf("terminated")
		}

		err := item.(func() error)()
		if err != nil {
			return err
		}
	}
}

func (m *hlsMuxer) handleRequest(req *hlsMuxerRequest) func() *hls.MuxerFileResponse {
	atomic.StoreInt64(m.lastRequestTime, time.Now().UnixNano())

	err := m.authenticate(req.ctx)
	if err != nil {
		if terr, ok := err.(pathErrAuthCritical); ok {
			m.log(logger.Info, "authentication error: %s", terr.message)
			return func() *hls.MuxerFileResponse {
				return &hls.MuxerFileResponse{
					Status: http.StatusUnauthorized,
					Header: map[string]string{
						"WWW-Authenticate": `Basic realm="rtsp-simple-server"`,
					},
				}
			}
		}

		return func() *hls.MuxerFileResponse {
			return &hls.MuxerFileResponse{
				Status: http.StatusUnauthorized,
				Header: map[string]string{
					"WWW-Authenticate": `Basic realm="rtsp-simple-server"`,
				},
			}
		}
	}

	if req.file == "" {
		return func() *hls.MuxerFileResponse {
			return &hls.MuxerFileResponse{
				Status: http.StatusOK,
				Header: map[string]string{
					"Content-Type": `text/html`,
				},
				Body: bytes.NewReader(hlsIndex),
			}
		}
	}

	return func() *hls.MuxerFileResponse {
		return m.muxer.File(
			req.file,
			req.ctx.Query("_HLS_msn"),
			req.ctx.Query("_HLS_part"),
			req.ctx.Query("_HLS_skip"))
	}
}

func (m *hlsMuxer) authenticate(ctx *gin.Context) error {
	pathConf := m.path.Conf()
	pathIPs := pathConf.ReadIPs
	pathUser := pathConf.ReadUser
	pathPass := pathConf.ReadPass

	if m.externalAuthenticationURL != "" {
		ip := net.ParseIP(ctx.ClientIP())
		user, pass, ok := ctx.Request.BasicAuth()

		err := externalAuth(
			m.externalAuthenticationURL,
			ip.String(),
			user,
			pass,
			m.pathName,
			false,
			ctx.Request.URL.RawQuery)
		if err != nil {
			if !ok {
				return pathErrAuthNotCritical{}
			}

			return pathErrAuthCritical{
				message: fmt.Sprintf("external authentication failed: %s", err),
			}
		}
	}

	if pathIPs != nil {
		ip := net.ParseIP(ctx.ClientIP())

		if !ipEqualOrInRange(ip, pathIPs) {
			return pathErrAuthCritical{
				message: fmt.Sprintf("IP '%s' not allowed", ip),
			}
		}
	}

	if pathUser != "" {
		user, pass, ok := ctx.Request.BasicAuth()
		if !ok {
			return pathErrAuthNotCritical{}
		}

		if user != string(pathUser) || pass != string(pathPass) {
			return pathErrAuthCritical{
				message: "invalid credentials",
			}
		}
	}

	return nil
}

func (m *hlsMuxer) addSentBytes(n uint64) {
	atomic.AddUint64(m.bytesSent, n)
}

// request is called by hlsserver.Server (forwarded from ServeHTTP).
func (m *hlsMuxer) request(req *hlsMuxerRequest) {
	select {
	case m.chRequest <- req:
	case <-m.ctx.Done():
		req.res <- hlsMuxerResponse{
			muxer: m,
			cb: func() *hls.MuxerFileResponse {
				return &hls.MuxerFileResponse{Status: http.StatusInternalServerError}
			},
		}
	}
}

// apiMuxersList is called by api.
func (m *hlsMuxer) apiMuxersList(req hlsServerAPIMuxersListSubReq) {
	req.res = make(chan struct{})
	select {
	case m.chAPIHLSMuxersList <- req:
		<-req.res

	case <-m.ctx.Done():
	}
}

// apiReaderDescribe implements reader.
func (m *hlsMuxer) apiReaderDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"hlsMuxer"}
}
