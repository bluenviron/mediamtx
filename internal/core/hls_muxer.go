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
	"github.com/aler9/rtsp-simple-server/internal/hls"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

const (
	closeCheckPeriod     = 1 * time.Second
	closeAfterInactivity = 60 * time.Second
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
	hlsVariant                conf.HLSVariant
	hlsSegmentCount           int
	hlsSegmentDuration        conf.StringDuration
	hlsPartDuration           conf.StringDuration
	hlsSegmentMaxSize         conf.StringSize
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
	hlsVariant conf.HLSVariant,
	hlsSegmentCount int,
	hlsSegmentDuration conf.StringDuration,
	hlsPartDuration conf.StringDuration,
	hlsSegmentMaxSize conf.StringSize,
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
		hlsVariant:                hlsVariant,
		hlsSegmentCount:           hlsSegmentCount,
		hlsSegmentDuration:        hlsSegmentDuration,
		hlsPartDuration:           hlsPartDuration,
		hlsSegmentMaxSize:         hlsSegmentMaxSize,
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

	innerCtx, innerCtxCancel := context.WithCancel(context.Background())
	innerReady := make(chan struct{})
	innerErr := make(chan error)
	go func() {
		innerErr <- m.runInner(innerCtx, innerReady)
	}()

	isReady := false

	err := func() error {
		for {
			select {
			case <-m.ctx.Done():
				innerCtxCancel()
				<-innerErr
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
				return err
			}
		}
	}()

	m.ctxCancel()

	for _, req := range m.requests {
		req.res <- hlsMuxerResponse{
			muxer: m,
			cb: func() *hls.MuxerFileResponse {
				return &hls.MuxerFileResponse{Status: http.StatusNotFound}
			},
		}
	}

	m.parent.muxerClose(m)

	m.log(logger.Info, "destroyed (%v)", err)
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

	var videoFormat *format.H264
	videoMedia := res.stream.medias().FindFormat(&videoFormat)

	var audioFormat *format.MPEG4Audio
	audioMedia := res.stream.medias().FindFormat(&audioFormat)

	if videoFormat == nil && audioFormat == nil {
		return fmt.Errorf("the stream doesn't contain an H264 track or an AAC track")
	}

	var err error
	m.muxer, err = hls.NewMuxer(
		hls.MuxerVariant(m.hlsVariant),
		m.hlsSegmentCount,
		time.Duration(m.hlsSegmentDuration),
		time.Duration(m.hlsPartDuration),
		uint64(m.hlsSegmentMaxSize),
		videoFormat,
		audioFormat,
	)
	if err != nil {
		return fmt.Errorf("muxer error: %v", err)
	}
	defer m.muxer.Close()

	innerReady <- struct{}{}

	m.ringBuffer, _ = ringbuffer.New(uint64(m.readBufferCount))

	var medias media.Medias

	if videoMedia != nil {
		medias = append(medias, videoMedia)

		videoStartPTSFilled := false
		var videoStartPTS time.Duration

		res.stream.readerAdd(m, videoMedia, videoFormat, func(dat data) {
			m.ringBuffer.Push(func() error {
				tdata := dat.(*dataH264)

				if tdata.nalus == nil {
					return nil
				}

				if !videoStartPTSFilled {
					videoStartPTSFilled = true
					videoStartPTS = tdata.pts
				}
				pts := tdata.pts - videoStartPTS

				err := m.muxer.WriteH264(tdata.ntp, pts, tdata.nalus)
				if err != nil {
					return fmt.Errorf("muxer error: %v", err)
				}

				return nil
			})
		})
	}

	if audioMedia != nil {
		medias = append(medias, audioMedia)

		audioStartPTSFilled := false
		var audioStartPTS time.Duration

		res.stream.readerAdd(m, audioMedia, audioFormat, func(dat data) {
			m.ringBuffer.Push(func() error {
				tdata := dat.(*dataMPEG4Audio)

				if tdata.aus == nil {
					return nil
				}

				if !audioStartPTSFilled {
					audioStartPTSFilled = true
					audioStartPTS = tdata.pts
				}
				pts := tdata.pts - audioStartPTS

				for i, au := range tdata.aus {
					err := m.muxer.WriteAAC(
						tdata.ntp,
						pts+time.Duration(i)*mpeg4audio.SamplesPerAccessUnit*
							time.Second/time.Duration(audioFormat.ClockRate()),
						au)
					if err != nil {
						return fmt.Errorf("muxer error: %v", err)
					}
				}

				return nil
			})
		})
	}

	defer res.stream.readerRemove(m)

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
			t := time.Unix(0, atomic.LoadInt64(m.lastRequestTime))
			if m.remoteAddr != "" && time.Since(t) >= closeAfterInactivity {
				m.ringBuffer.Close()
				<-writerDone
				return fmt.Errorf("not used anymore")
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
