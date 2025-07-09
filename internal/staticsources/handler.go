// Package staticsources contains static source implementations.
package staticsources

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	sshls "github.com/bluenviron/mediamtx/internal/staticsources/hls"
	ssrpicamera "github.com/bluenviron/mediamtx/internal/staticsources/rpicamera"
	ssrtmp "github.com/bluenviron/mediamtx/internal/staticsources/rtmp"
	ssrtsp "github.com/bluenviron/mediamtx/internal/staticsources/rtsp"
	sssrt "github.com/bluenviron/mediamtx/internal/staticsources/srt"
	ssudp "github.com/bluenviron/mediamtx/internal/staticsources/udp"
	sswebrtc "github.com/bluenviron/mediamtx/internal/staticsources/webrtc"
	"github.com/bluenviron/mediamtx/internal/stream"
)

const (
	retryPause = 5 * time.Second
)

func emptyTimer() *time.Timer {
	t := time.NewTimer(0)
	<-t.C
	return t
}

func resolveSource(s string, matches []string, query string) string {
	if len(matches) > 1 {
		for i, ma := range matches[1:] {
			s = strings.ReplaceAll(s, "$G"+strconv.FormatInt(int64(i+1), 10), ma)
		}
	}

	s = strings.ReplaceAll(s, "$MTX_QUERY", query)

	return s
}

type handlerPathManager interface {
	AddReader(req defs.PathAddReaderReq) (defs.Path, *stream.Stream, error)
}

type handlerParent interface {
	logger.Writer
	StaticSourceHandlerSetReady(context.Context, defs.PathSourceStaticSetReadyReq)
	StaticSourceHandlerSetNotReady(context.Context, defs.PathSourceStaticSetNotReadyReq)
}

// Handler is a static source handler.
type Handler struct {
	Conf              *conf.Path
	LogLevel          conf.LogLevel
	ReadTimeout       conf.Duration
	WriteTimeout      conf.Duration
	WriteQueueSize    int
	RTPMaxPayloadSize int
	Matches           []string
	PathManager       handlerPathManager
	Parent            handlerParent

	ctx       context.Context
	ctxCancel func()
	instance  defs.StaticSource
	running   bool
	query     string

	// in
	chReloadConf          chan *conf.Path
	chInstanceSetReady    chan defs.PathSourceStaticSetReadyReq
	chInstanceSetNotReady chan defs.PathSourceStaticSetNotReadyReq

	// out
	done chan struct{}
}

// Initialize initializes Handler.
func (s *Handler) Initialize() {
	s.chReloadConf = make(chan *conf.Path)
	s.chInstanceSetReady = make(chan defs.PathSourceStaticSetReadyReq)
	s.chInstanceSetNotReady = make(chan defs.PathSourceStaticSetNotReadyReq)

	switch {
	case strings.HasPrefix(s.Conf.Source, "rtsp://") ||
		strings.HasPrefix(s.Conf.Source, "rtsps://"):
		s.instance = &ssrtsp.Source{
			ReadTimeout:    s.ReadTimeout,
			WriteTimeout:   s.WriteTimeout,
			WriteQueueSize: s.WriteQueueSize,
			Parent:         s,
		}

	case strings.HasPrefix(s.Conf.Source, "rtmp://") ||
		strings.HasPrefix(s.Conf.Source, "rtmps://"):
		s.instance = &ssrtmp.Source{
			ReadTimeout:  s.ReadTimeout,
			WriteTimeout: s.WriteTimeout,
			Parent:       s,
		}

	case strings.HasPrefix(s.Conf.Source, "http://") ||
		strings.HasPrefix(s.Conf.Source, "https://"):
		s.instance = &sshls.Source{
			ReadTimeout: s.ReadTimeout,
			Parent:      s,
		}

	case strings.HasPrefix(s.Conf.Source, "udp://"):
		s.instance = &ssudp.Source{
			ReadTimeout: s.ReadTimeout,
			Parent:      s,
		}

	case strings.HasPrefix(s.Conf.Source, "srt://"):
		s.instance = &sssrt.Source{
			ReadTimeout: s.ReadTimeout,
			Parent:      s,
		}

	case strings.HasPrefix(s.Conf.Source, "whep://") ||
		strings.HasPrefix(s.Conf.Source, "wheps://"):
		s.instance = &sswebrtc.Source{
			ReadTimeout: s.ReadTimeout,
			Parent:      s,
		}

	case s.Conf.Source == "rpiCamera":
		s.instance = &ssrpicamera.Source{
			RTPMaxPayloadSize: s.RTPMaxPayloadSize,
			LogLevel:          s.LogLevel,
			Parent:            s,
		}

	default:
		panic("should not happen")
	}
}

// Close closes Handler.
func (s *Handler) Close(reason string) {
	s.Stop(reason)
}

// Start starts Handler.
func (s *Handler) Start(onDemand bool, query string) {
	if s.running {
		panic("should not happen")
	}

	s.running = true
	s.query = query
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	s.done = make(chan struct{})

	s.instance.Log(logger.Info, "started%s",
		func() string {
			if onDemand {
				return " on demand"
			}
			return ""
		}())

	go s.run()
}

// Stop stops Handler.
func (s *Handler) Stop(reason string) {
	if !s.running {
		panic("should not happen")
	}

	s.running = false

	s.instance.Log(logger.Info, "stopped: %s", reason)

	s.ctxCancel()

	// we must wait since s.ctx is not thread safe
	<-s.done
}

// Log implements logger.Writer.
func (s *Handler) Log(level logger.Level, format string, args ...interface{}) {
	s.Parent.Log(level, format, args...)
}

func (s *Handler) run() {
	defer close(s.done)

	var runCtx context.Context
	var runCtxCancel func()
	runErr := make(chan error)
	runReloadConf := make(chan *conf.Path)

	recreate := func() {
		resolvedSource := resolveSource(s.Conf.Source, s.Matches, s.query)

		runCtx, runCtxCancel = context.WithCancel(context.Background())
		go func() {
			runErr <- s.instance.Run(defs.StaticSourceRunParams{
				Context:        runCtx,
				ResolvedSource: resolvedSource,
				Conf:           s.Conf,
				ReloadConf:     runReloadConf,
			})
		}()
	}

	recreate()

	recreating := false
	recreateTimer := emptyTimer()

	for {
		select {
		case err := <-runErr:
			runCtxCancel()
			s.instance.Log(logger.Error, err.Error())
			recreating = true
			recreateTimer = time.NewTimer(retryPause)

		case req := <-s.chInstanceSetReady:
			s.Parent.StaticSourceHandlerSetReady(s.ctx, req)

		case req := <-s.chInstanceSetNotReady:
			s.Parent.StaticSourceHandlerSetNotReady(s.ctx, req)

		case newConf := <-s.chReloadConf:
			s.Conf = newConf
			if !recreating {
				cReloadConf := runReloadConf
				cInnerCtx := runCtx
				go func() {
					select {
					case cReloadConf <- newConf:
					case <-cInnerCtx.Done():
					}
				}()
			}

		case <-recreateTimer.C:
			recreate()
			recreating = false

		case <-s.ctx.Done():
			if !recreating {
				runCtxCancel()
				<-runErr
			}
			return
		}
	}
}

// ReloadConf is called by path.
func (s *Handler) ReloadConf(newConf *conf.Path) {
	ctx := s.ctx

	if !s.running {
		return
	}

	go func() {
		select {
		case s.chReloadConf <- newConf:
		case <-ctx.Done():
		}
	}()
}

// APISourceDescribe instanceements source.
func (s *Handler) APISourceDescribe() defs.APIPathSourceOrReader {
	return s.instance.APISourceDescribe()
}

// SetReady is called by a staticSource.
func (s *Handler) SetReady(req defs.PathSourceStaticSetReadyReq) defs.PathSourceStaticSetReadyRes {
	req.Res = make(chan defs.PathSourceStaticSetReadyRes)
	select {
	case s.chInstanceSetReady <- req:
		res := <-req.Res

		if res.Err == nil {
			s.instance.Log(logger.Info, "ready: %s", defs.MediasInfo(req.Desc.Medias))
		}

		return res

	case <-s.ctx.Done():
		return defs.PathSourceStaticSetReadyRes{Err: fmt.Errorf("terminated")}
	}
}

// SetNotReady is called by a staticSource.
func (s *Handler) SetNotReady(req defs.PathSourceStaticSetNotReadyReq) {
	req.Res = make(chan struct{})
	select {
	case s.chInstanceSetNotReady <- req:
		<-req.Res
	case <-s.ctx.Done():
	}
}

// AddReader is called by a staticSource.
func (s *Handler) AddReader(req defs.PathAddReaderReq) (defs.Path, *stream.Stream, error) {
	return s.PathManager.AddReader(req)
}
