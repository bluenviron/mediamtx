package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

const (
	sourceStaticRetryPause = 5 * time.Second
)

type sourceStaticImpl interface {
	Log(logger.Level, string, ...interface{})
	run(context.Context, *conf.PathConf, chan *conf.PathConf) error
	apiSourceDescribe() interface{}
}

type sourceStaticParent interface {
	log(logger.Level, string, ...interface{})
	sourceStaticSetReady(context.Context, pathSourceStaticSetReadyReq)
	sourceStaticSetNotReady(context.Context, pathSourceStaticSetNotReadyReq)
}

// sourceStatic is a static source.
type sourceStatic struct {
	conf   *conf.PathConf
	parent sourceStaticParent

	ctx       context.Context
	ctxCancel func()
	impl      sourceStaticImpl
	running   bool

	// in
	chReloadConf                  chan *conf.PathConf
	chSourceStaticImplSetReady    chan pathSourceStaticSetReadyReq
	chSourceStaticImplSetNotReady chan pathSourceStaticSetNotReadyReq

	// out
	done chan struct{}
}

func newSourceStatic(
	cnf *conf.PathConf,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
	parent sourceStaticParent,
) *sourceStatic {
	s := &sourceStatic{
		conf:                          cnf,
		parent:                        parent,
		chReloadConf:                  make(chan *conf.PathConf),
		chSourceStaticImplSetReady:    make(chan pathSourceStaticSetReadyReq),
		chSourceStaticImplSetNotReady: make(chan pathSourceStaticSetNotReadyReq),
	}

	switch {
	case strings.HasPrefix(cnf.Source, "rtsp://") ||
		strings.HasPrefix(cnf.Source, "rtsps://"):
		s.impl = newRTSPSource(
			readTimeout,
			writeTimeout,
			readBufferCount,
			s)

	case strings.HasPrefix(cnf.Source, "rtmp://") ||
		strings.HasPrefix(cnf.Source, "rtmps://"):
		s.impl = newRTMPSource(
			readTimeout,
			writeTimeout,
			s)

	case strings.HasPrefix(cnf.Source, "http://") ||
		strings.HasPrefix(cnf.Source, "https://"):
		s.impl = newHLSSource(
			s)

	case strings.HasPrefix(cnf.Source, "udp://"):
		s.impl = newUDPSource(
			readTimeout,
			s)

	case cnf.Source == "rpiCamera":
		s.impl = newRPICameraSource(
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

	var innerCtx context.Context
	var innerCtxCancel func()
	implErr := make(chan error)
	innerReloadConf := make(chan *conf.PathConf)

	recreate := func() {
		innerCtx, innerCtxCancel = context.WithCancel(context.Background())
		go func() {
			implErr <- s.impl.run(innerCtx, s.conf, innerReloadConf)
		}()
	}

	recreate()

	recreating := false
	recreateTimer := newEmptyTimer()

	for {
		select {
		case err := <-implErr:
			innerCtxCancel()
			s.impl.Log(logger.Info, "ERR: %v", err)
			recreating = true
			recreateTimer = time.NewTimer(sourceStaticRetryPause)

		case newConf := <-s.chReloadConf:
			s.conf = newConf
			if !recreating {
				cReloadConf := innerReloadConf
				cInnerCtx := innerCtx
				go func() {
					select {
					case cReloadConf <- newConf:
					case <-cInnerCtx.Done():
					}
				}()
			}

		case req := <-s.chSourceStaticImplSetReady:
			s.parent.sourceStaticSetReady(s.ctx, req)

		case req := <-s.chSourceStaticImplSetNotReady:
			s.parent.sourceStaticSetNotReady(s.ctx, req)

		case <-recreateTimer.C:
			recreate()
			recreating = false

		case <-s.ctx.Done():
			if !recreating {
				innerCtxCancel()
				<-implErr
			}
			return
		}
	}
}

func (s *sourceStatic) reloadConf(newConf *conf.PathConf) {
	select {
	case s.chReloadConf <- newConf:
	case <-s.ctx.Done():
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
