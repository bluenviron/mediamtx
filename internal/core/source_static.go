package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/rpicamera"
)

const (
	sourceStaticRetryPause = 5 * time.Second
)

type sourceStaticImpl interface {
	Log(logger.Level, string, ...interface{})
	run(context.Context) error
	apiSourceDescribe() interface{}
}

type sourceStaticParent interface {
	log(logger.Level, string, ...interface{})
	sourceStaticSetReady(context.Context, pathSourceStaticSetReadyReq)
	sourceStaticSetNotReady(context.Context, pathSourceStaticSetNotReadyReq)
}

// sourceStatic is a static source.
type sourceStatic struct {
	parent sourceStaticParent

	ctx       context.Context
	ctxCancel func()
	impl      sourceStaticImpl
	running   bool

	done                          chan struct{}
	chSourceStaticImplSetReady    chan pathSourceStaticSetReadyReq
	chSourceStaticImplSetNotReady chan pathSourceStaticSetNotReadyReq
}

func newSourceStatic(
	conf *conf.PathConf,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
	parent sourceStaticParent,
) *sourceStatic {
	s := &sourceStatic{
		parent:                        parent,
		chSourceStaticImplSetReady:    make(chan pathSourceStaticSetReadyReq),
		chSourceStaticImplSetNotReady: make(chan pathSourceStaticSetNotReadyReq),
	}

	switch {
	case strings.HasPrefix(conf.Source, "rtsp://") ||
		strings.HasPrefix(conf.Source, "rtsps://"):
		s.impl = newRTSPSource(
			conf.Source,
			conf.SourceProtocol,
			conf.SourceAnyPortEnable,
			conf.SourceFingerprint,
			readTimeout,
			writeTimeout,
			readBufferCount,
			s)

	case strings.HasPrefix(conf.Source, "rtmp://") ||
		strings.HasPrefix(conf.Source, "rtmps://"):
		s.impl = newRTMPSource(
			conf.Source,
			conf.SourceFingerprint,
			readTimeout,
			writeTimeout,
			s)

	case strings.HasPrefix(conf.Source, "http://") ||
		strings.HasPrefix(conf.Source, "https://"):
		s.impl = newHLSSource(
			conf.Source,
			conf.SourceFingerprint,
			s)

	case conf.Source == "rpiCamera":
		s.impl = newRPICameraSource(
			rpicamera.Params{
				CameraID:  conf.RPICameraCamID,
				Width:     conf.RPICameraWidth,
				Height:    conf.RPICameraHeight,
				FPS:       conf.RPICameraFPS,
				IDRPeriod: conf.RPICameraIDRPeriod,
				Bitrate:   conf.RPICameraBitrate,
				Profile:   conf.RPICameraProfile,
				Level:     conf.RPICameraLevel,
			},
			s)
	}

	return s
}

func (s *sourceStatic) close() {
	if s.running {
		s.stop()
	}
}

func (s *sourceStatic) start() {
	if s.running {
		panic("should not happen")
	}

	s.running = true
	s.impl.Log(logger.Info, "started")

	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	s.done = make(chan struct{})

	go s.run()
}

func (s *sourceStatic) stop() {
	if !s.running {
		panic("should not happen")
	}

	s.running = false
	s.impl.Log(logger.Info, "stopped")

	s.ctxCancel()

	// we must wait since s.ctx is not thread safe
	<-s.done
}

func (s *sourceStatic) log(level logger.Level, format string, args ...interface{}) {
	s.parent.log(level, format, args...)
}

func (s *sourceStatic) run() {
	defer close(s.done)

outer:
	for {
		s.runInner()

		select {
		case <-time.After(sourceStaticRetryPause):
		case <-s.ctx.Done():
			break outer
		}
	}

	s.ctxCancel()
}

func (s *sourceStatic) runInner() {
	innerCtx, innerCtxCancel := context.WithCancel(context.Background())
	implErr := make(chan error)
	go func() {
		implErr <- s.impl.run(innerCtx)
	}()

	for {
		select {
		case err := <-implErr:
			innerCtxCancel()
			s.impl.Log(logger.Info, "ERR: %v", err)
			return

		case req := <-s.chSourceStaticImplSetReady:
			s.parent.sourceStaticSetReady(s.ctx, req)

		case req := <-s.chSourceStaticImplSetNotReady:
			s.parent.sourceStaticSetNotReady(s.ctx, req)

		case <-s.ctx.Done():
			innerCtxCancel()
			<-implErr
			return
		}
	}
}

// apiSourceDescribe implements source.
func (s *sourceStatic) apiSourceDescribe() interface{} {
	return s.impl.apiSourceDescribe()
}

// sourceStaticImplSetReady is called by a sourceStaticImpl.
func (s *sourceStatic) sourceStaticImplSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes {
	req.res = make(chan pathSourceStaticSetReadyRes)
	select {
	case s.chSourceStaticImplSetReady <- req:
		return <-req.res
	case <-s.ctx.Done():
		return pathSourceStaticSetReadyRes{err: fmt.Errorf("terminated")}
	}
}

// sourceStaticImplSetNotReady is called by a sourceStaticImpl.
func (s *sourceStatic) sourceStaticImplSetNotReady(req pathSourceStaticSetNotReadyReq) {
	req.res = make(chan struct{})
	select {
	case s.chSourceStaticImplSetNotReady <- req:
		<-req.res
	case <-s.ctx.Done():
	}
}
