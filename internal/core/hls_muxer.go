package core

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gohlslib"
	"github.com/bluenviron/gohlslib/pkg/codecs"
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/bluenviron/gortsplib/v3/pkg/ringbuffer"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/formatprocessor"
	"github.com/bluenviron/mediamtx/internal/logger"
)

const (
	closeCheckPeriod      = 1 * time.Second
	closeAfterInactivity  = 60 * time.Second
	hlsMuxerRecreatePause = 10 * time.Second
)

func int64Ptr(v int64) *int64 {
	return &v
}

type responseWriterWithCounter struct {
	http.ResponseWriter
	bytesSent *uint64
}

func (w *responseWriterWithCounter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	atomic.AddUint64(w.bytesSent, uint64(n))
	return n, err
}

type hlsMuxerHandleRequestReq struct {
	path string
	file string
	ctx  *gin.Context
	res  chan *hlsMuxer
}

type hlsMuxerParent interface {
	logger.Writer
	muxerClose(*hlsMuxer)
}

type hlsMuxer struct {
	remoteAddr                string
	externalAuthenticationURL string
	alwaysRemux               bool
	variant                   conf.HLSVariant
	segmentCount              int
	segmentDuration           conf.StringDuration
	partDuration              conf.StringDuration
	segmentMaxSize            conf.StringSize
	directory                 string
	readBufferCount           int
	wg                        *sync.WaitGroup
	pathName                  string
	pathManager               *pathManager
	parent                    hlsMuxerParent

	ctx             context.Context
	ctxCancel       func()
	created         time.Time
	path            *path
	ringBuffer      *ringbuffer.RingBuffer
	lastRequestTime *int64
	muxer           *gohlslib.Muxer
	requests        []*hlsMuxerHandleRequestReq
	bytesSent       *uint64

	// in
	chRequest chan *hlsMuxerHandleRequestReq
}

func newHLSMuxer(
	parentCtx context.Context,
	remoteAddr string,
	externalAuthenticationURL string,
	alwaysRemux bool,
	variant conf.HLSVariant,
	segmentCount int,
	segmentDuration conf.StringDuration,
	partDuration conf.StringDuration,
	segmentMaxSize conf.StringSize,
	directory string,
	readBufferCount int,
	wg *sync.WaitGroup,
	pathName string,
	pathManager *pathManager,
	parent hlsMuxerParent,
) *hlsMuxer {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	m := &hlsMuxer{
		remoteAddr:                remoteAddr,
		externalAuthenticationURL: externalAuthenticationURL,
		alwaysRemux:               alwaysRemux,
		variant:                   variant,
		segmentCount:              segmentCount,
		segmentDuration:           segmentDuration,
		partDuration:              partDuration,
		segmentMaxSize:            segmentMaxSize,
		directory:                 directory,
		readBufferCount:           readBufferCount,
		wg:                        wg,
		pathName:                  pathName,
		pathManager:               pathManager,
		parent:                    parent,
		ctx:                       ctx,
		ctxCancel:                 ctxCancel,
		created:                   time.Now(),
		lastRequestTime:           int64Ptr(time.Now().UnixNano()),
		bytesSent:                 new(uint64),
		chRequest:                 make(chan *hlsMuxerHandleRequestReq),
	}

	m.Log(logger.Info, "created %s", func() string {
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

func (m *hlsMuxer) Log(level logger.Level, format string, args ...interface{}) {
	m.parent.Log(level, "[muxer %s] "+format, append([]interface{}{m.pathName}, args...)...)
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
				switch {
				case isRecreating:
					req.res <- nil

				case isReady:
					req.res <- m

				default:
					m.requests = append(m.requests, req)
				}

			case <-innerReady:
				isReady = true
				for _, req := range m.requests {
					req.res <- m
				}
				m.requests = nil

			case err := <-innerErr:
				innerCtxCancel()

				if m.alwaysRemux {
					m.Log(logger.Info, "ERR: %v", err)
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

	m.Log(logger.Info, "destroyed (%v)", err)
}

func (m *hlsMuxer) clearQueuedRequests() {
	for _, req := range m.requests {
		req.res <- nil
	}
	m.requests = nil
}

func (m *hlsMuxer) runInner(innerCtx context.Context, innerReady chan struct{}) error {
	res := m.pathManager.readerAdd(pathReaderAddReq{
		author:   m,
		pathName: m.pathName,
		skipAuth: true,
	})
	if res.err != nil {
		return res.err
	}

	m.path = res.path

	defer m.path.readerRemove(pathReaderRemoveReq{author: m})

	m.ringBuffer, _ = ringbuffer.New(uint64(m.readBufferCount))

	var medias media.Medias

	videoMedia, videoTrack := m.createVideoTrack(res.stream)
	if videoMedia != nil {
		medias = append(medias, videoMedia)
	}

	audioMedia, audioTrack := m.createAudioTrack(res.stream)
	if audioMedia != nil {
		medias = append(medias, audioMedia)
	}

	defer res.stream.readerRemove(m)

	if medias == nil {
		return fmt.Errorf(
			"the stream doesn't contain any supported codec, which are currently H264, H265, MPEG4-Audio, Opus")
	}

	var muxerDirectory string
	if m.directory != "" {
		muxerDirectory = filepath.Join(m.directory, m.pathName)
		os.MkdirAll(muxerDirectory, 0o755)
		defer os.Remove(muxerDirectory)
	}

	m.muxer = &gohlslib.Muxer{
		Variant:         gohlslib.MuxerVariant(m.variant),
		SegmentCount:    m.segmentCount,
		SegmentDuration: time.Duration(m.segmentDuration),
		PartDuration:    time.Duration(m.partDuration),
		SegmentMaxSize:  uint64(m.segmentMaxSize),
		VideoTrack:      videoTrack,
		AudioTrack:      audioTrack,
		Directory:       muxerDirectory,
	}

	err := m.muxer.Start()
	if err != nil {
		return fmt.Errorf("muxer error: %v", err)
	}
	defer m.muxer.Close()

	innerReady <- struct{}{}

	m.Log(logger.Info, "is converting into HLS, %s",
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

func (m *hlsMuxer) createVideoTrack(stream *stream) (*media.Media, *gohlslib.Track) {
	var videoFormatH265 *formats.H265
	videoMedia := stream.medias().FindFormat(&videoFormatH265)

	if videoFormatH265 != nil {
		videoStartPTSFilled := false
		var videoStartPTS time.Duration

		stream.readerAdd(m, videoMedia, videoFormatH265, func(unit formatprocessor.Unit) {
			m.ringBuffer.Push(func() error {
				tunit := unit.(*formatprocessor.UnitH265)

				if tunit.AU == nil {
					return nil
				}

				if !videoStartPTSFilled {
					videoStartPTSFilled = true
					videoStartPTS = tunit.PTS
				}
				pts := tunit.PTS - videoStartPTS

				err := m.muxer.WriteH26x(tunit.NTP, pts, tunit.AU)
				if err != nil {
					return fmt.Errorf("muxer error: %v", err)
				}

				return nil
			})
		})

		vps, sps, pps := videoFormatH265.SafeParams()

		return videoMedia, &gohlslib.Track{
			Codec: &codecs.H265{
				VPS: vps,
				SPS: sps,
				PPS: pps,
			},
		}
	}

	var videoFormatH264 *formats.H264
	videoMedia = stream.medias().FindFormat(&videoFormatH264)

	if videoFormatH264 != nil {
		videoStartPTSFilled := false
		var videoStartPTS time.Duration

		stream.readerAdd(m, videoMedia, videoFormatH264, func(unit formatprocessor.Unit) {
			m.ringBuffer.Push(func() error {
				tunit := unit.(*formatprocessor.UnitH264)

				if tunit.AU == nil {
					return nil
				}

				if !videoStartPTSFilled {
					videoStartPTSFilled = true
					videoStartPTS = tunit.PTS
				}
				pts := tunit.PTS - videoStartPTS

				err := m.muxer.WriteH26x(tunit.NTP, pts, tunit.AU)
				if err != nil {
					return fmt.Errorf("muxer error: %v", err)
				}

				return nil
			})
		})

		sps, pps := videoFormatH264.SafeParams()

		return videoMedia, &gohlslib.Track{
			Codec: &codecs.H264{
				SPS: sps,
				PPS: pps,
			},
		}
	}

	return nil, nil
}

func (m *hlsMuxer) createAudioTrack(stream *stream) (*media.Media, *gohlslib.Track) {
	var audioFormatMPEG4Audio *formats.MPEG4Audio
	audioMedia := stream.medias().FindFormat(&audioFormatMPEG4Audio)

	if audioFormatMPEG4Audio != nil {
		audioStartPTSFilled := false
		var audioStartPTS time.Duration

		stream.readerAdd(m, audioMedia, audioFormatMPEG4Audio, func(unit formatprocessor.Unit) {
			m.ringBuffer.Push(func() error {
				tunit := unit.(*formatprocessor.UnitMPEG4Audio)

				if tunit.AUs == nil {
					return nil
				}

				if !audioStartPTSFilled {
					audioStartPTSFilled = true
					audioStartPTS = tunit.PTS
				}
				pts := tunit.PTS - audioStartPTS

				for i, au := range tunit.AUs {
					err := m.muxer.WriteAudio(
						tunit.NTP,
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

		return audioMedia, &gohlslib.Track{
			Codec: &codecs.MPEG4Audio{
				Config: *audioFormatMPEG4Audio.Config,
			},
		}
	}

	var audioFormatOpus *formats.Opus
	audioMedia = stream.medias().FindFormat(&audioFormatOpus)

	if audioFormatOpus != nil {
		audioStartPTSFilled := false
		var audioStartPTS time.Duration

		stream.readerAdd(m, audioMedia, audioFormatOpus, func(unit formatprocessor.Unit) {
			m.ringBuffer.Push(func() error {
				tunit := unit.(*formatprocessor.UnitOpus)

				if !audioStartPTSFilled {
					audioStartPTSFilled = true
					audioStartPTS = tunit.PTS
				}
				pts := tunit.PTS - audioStartPTS

				err := m.muxer.WriteAudio(
					tunit.NTP,
					pts,
					tunit.Frame)
				if err != nil {
					return fmt.Errorf("muxer error: %v", err)
				}

				return nil
			})
		})

		return audioMedia, &gohlslib.Track{
			Codec: &codecs.Opus{
				Channels: func() int {
					if audioFormatOpus.IsStereo {
						return 2
					}
					return 1
				}(),
			},
		}
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

func (m *hlsMuxer) handleRequest(ctx *gin.Context) {
	atomic.StoreInt64(m.lastRequestTime, time.Now().UnixNano())

	w := &responseWriterWithCounter{
		ResponseWriter: ctx.Writer,
		bytesSent:      m.bytesSent,
	}

	m.muxer.Handle(w, ctx.Request)
}

// processRequest is called by hlsserver.Server (forwarded from ServeHTTP).
func (m *hlsMuxer) processRequest(req *hlsMuxerHandleRequestReq) {
	select {
	case m.chRequest <- req:
	case <-m.ctx.Done():
		req.res <- nil
	}
}

// apiReaderDescribe implements reader.
func (m *hlsMuxer) apiReaderDescribe() pathAPISourceOrReader {
	return pathAPISourceOrReader{
		Type: "hlsMuxer",
		ID:   "",
	}
}

func (m *hlsMuxer) apiItem() *apiHLSMuxer {
	return &apiHLSMuxer{
		Path:        m.pathName,
		Created:     m.created,
		LastRequest: time.Unix(0, atomic.LoadInt64(m.lastRequestTime)),
		BytesSent:   atomic.LoadUint64(m.bytesSent),
	}
}
