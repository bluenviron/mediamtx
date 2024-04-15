package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	hlssource "github.com/bluenviron/mediamtx/internal/staticsources/hls"
	rpicamerasource "github.com/bluenviron/mediamtx/internal/staticsources/rpicamera"
	rtmpsource "github.com/bluenviron/mediamtx/internal/staticsources/rtmp"
	rtspsource "github.com/bluenviron/mediamtx/internal/staticsources/rtsp"
	srtsource "github.com/bluenviron/mediamtx/internal/staticsources/srt"
	udpsource "github.com/bluenviron/mediamtx/internal/staticsources/udp"
	webrtcsource "github.com/bluenviron/mediamtx/internal/staticsources/webrtc"
)

const (
	staticSourceHandlerRetryPause = 5 * time.Second
)

type staticSourceHandlerParent interface {
	logger.Writer
	staticSourceHandlerSetReady(context.Context, defs.PathSourceStaticSetReadyReq)
	staticSourceHandlerSetNotReady(context.Context, defs.PathSourceStaticSetNotReadyReq)
}

// staticSourceHandler is a static source handler.
type staticSourceHandler struct {
	conf           *conf.Path
	logLevel       conf.LogLevel
	readTimeout    conf.StringDuration
	writeTimeout   conf.StringDuration
	writeQueueSize int
	resolvedSource string
	parent         staticSourceHandlerParent

	ctx       context.Context
	ctxCancel func()
	instance  defs.StaticSource
	running   bool

	// in
	chReloadConf          chan *conf.Path
	chInstanceSetReady    chan defs.PathSourceStaticSetReadyReq
	chInstanceSetNotReady chan defs.PathSourceStaticSetNotReadyReq

	// out
	done chan struct{}
}

func (s *staticSourceHandler) initialize() {
	s.chReloadConf = make(chan *conf.Path)
	s.chInstanceSetReady = make(chan defs.PathSourceStaticSetReadyReq)
	s.chInstanceSetNotReady = make(chan defs.PathSourceStaticSetNotReadyReq)

	switch {
	case strings.HasPrefix(s.resolvedSource, "rtsp://") ||
		strings.HasPrefix(s.resolvedSource, "rtsps://"):
		s.instance = &rtspsource.Source{
			ResolvedSource: s.resolvedSource,
			ReadTimeout:    s.readTimeout,
			WriteTimeout:   s.writeTimeout,
			WriteQueueSize: s.writeQueueSize,
			Parent:         s,
		}

	case strings.HasPrefix(s.resolvedSource, "rtmp://") ||
		strings.HasPrefix(s.resolvedSource, "rtmps://"):
		s.instance = &rtmpsource.Source{
			ResolvedSource: s.resolvedSource,
			ReadTimeout:    s.readTimeout,
			WriteTimeout:   s.writeTimeout,
			Parent:         s,
		}

	case strings.HasPrefix(s.resolvedSource, "http://") ||
		strings.HasPrefix(s.resolvedSource, "https://"):
		s.instance = &hlssource.Source{
			ResolvedSource: s.resolvedSource,
			ReadTimeout:    s.readTimeout,
			Parent:         s,
		}

	case strings.HasPrefix(s.resolvedSource, "udp://"):
		s.instance = &udpsource.Source{
			ResolvedSource: s.resolvedSource,
			ReadTimeout:    s.readTimeout,
			Parent:         s,
		}

	case strings.HasPrefix(s.resolvedSource, "srt://"):
		s.instance = &srtsource.Source{
			ResolvedSource: s.resolvedSource,
			ReadTimeout:    s.readTimeout,
			Parent:         s,
		}

	case strings.HasPrefix(s.resolvedSource, "whep://") ||
		strings.HasPrefix(s.resolvedSource, "wheps://"):
		s.instance = &webrtcsource.Source{
			ResolvedSource: s.resolvedSource,
			ReadTimeout:    s.readTimeout,
			Parent:         s,
		}

	case s.resolvedSource == "rpiCamera":
		s.instance = &rpicamerasource.Source{
			LogLevel: s.logLevel,
			Parent:   s,
		}
	}
}

func (s *staticSourceHandler) close(reason string) {
	s.stop(reason)
}

func (s *staticSourceHandler) start(onDemand bool) {
	if s.running {
		panic("should not happen")
	}

	s.running = true
	s.instance.Log(logger.Info, "started%s",
		func() string {
			if onDemand {
				return " on demand"
			}
			return ""
		}())

	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	s.done = make(chan struct{})

	go s.run()
}

func (s *staticSourceHandler) stop(reason string) {
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
func (s *staticSourceHandler) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, format, args...)
}

func (s *staticSourceHandler) run() {
	defer close(s.done)

	var runCtx context.Context
	var runCtxCancel func()
	runErr := make(chan error)
	runReloadConf := make(chan *conf.Path)

	recreate := func() {
		runCtx, runCtxCancel = context.WithCancel(context.Background())
		go func() {
			runErr <- s.instance.Run(defs.StaticSourceRunParams{
				Context:    runCtx,
				Conf:       s.conf,
				ReloadConf: runReloadConf,
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
			recreateTimer = time.NewTimer(staticSourceHandlerRetryPause)

		case req := <-s.chInstanceSetReady:
			s.parent.staticSourceHandlerSetReady(s.ctx, req)

		case req := <-s.chInstanceSetNotReady:
			s.parent.staticSourceHandlerSetNotReady(s.ctx, req)

		case newConf := <-s.chReloadConf:
			s.conf = newConf
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

func (s *staticSourceHandler) reloadConf(newConf *conf.Path) {
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
func (s *staticSourceHandler) APISourceDescribe() defs.APIPathSourceOrReader {
	return s.instance.APISourceDescribe()
}

// setReady is called by a staticSource.
func (s *staticSourceHandler) SetReady(req defs.PathSourceStaticSetReadyReq) defs.PathSourceStaticSetReadyRes {
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

// setNotReady is called by a staticSource.
func (s *staticSourceHandler) SetNotReady(req defs.PathSourceStaticSetNotReadyReq) {
	req.Res = make(chan struct{})
	select {
	case s.chInstanceSetNotReady <- req:
		<-req.Res
	case <-s.ctx.Done():
	}
}
