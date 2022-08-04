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
	run(context.Context) error
	onSourceAPIDescribe() interface{}
}

type sourceStaticParent interface {
	log(logger.Level, string, ...interface{})
	onSourceStaticSetReady(context.Context, pathSourceStaticSetReadyReq)
	onSourceStaticSetNotReady(context.Context, pathSourceStaticSetNotReadyReq)
}

// sourceStatic is a static source.
type sourceStatic struct {
	ur              string
	protocol        conf.SourceProtocol
	anyPortEnable   bool
	fingerprint     string
	readTimeout     conf.StringDuration
	writeTimeout    conf.StringDuration
	readBufferCount int
	parent          sourceStaticParent

	ctx       context.Context
	ctxCancel func()
	impl      sourceStaticImpl
	running   bool

	done                        chan struct{}
	sourceStaticImplSetReady    chan pathSourceStaticSetReadyReq
	sourceStaticImplSetNotReady chan pathSourceStaticSetNotReadyReq
}

func newSourceStatic(
	ur string,
	protocol conf.SourceProtocol,
	anyPortEnable bool,
	fingerprint string,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
	parent sourceStaticParent,
) *sourceStatic {
	s := &sourceStatic{
		ur:                          ur,
		protocol:                    protocol,
		anyPortEnable:               anyPortEnable,
		fingerprint:                 fingerprint,
		readTimeout:                 readTimeout,
		writeTimeout:                writeTimeout,
		readBufferCount:             readBufferCount,
		parent:                      parent,
		sourceStaticImplSetReady:    make(chan pathSourceStaticSetReadyReq),
		sourceStaticImplSetNotReady: make(chan pathSourceStaticSetNotReadyReq),
	}

	switch {
	case strings.HasPrefix(s.ur, "rtsp://") ||
		strings.HasPrefix(s.ur, "rtsps://"):
		s.impl = newRTSPSource(
			s.ur,
			s.protocol,
			s.anyPortEnable,
			s.fingerprint,
			s.readTimeout,
			s.writeTimeout,
			s.readBufferCount,
			s)

	case strings.HasPrefix(s.ur, "rtmp://"):
		s.impl = newRTMPSource(
			s.ur,
			s.readTimeout,
			s.writeTimeout,
			s)

	case strings.HasPrefix(s.ur, "http://") ||
		strings.HasPrefix(s.ur, "https://"):
		s.impl = newHLSSource(
			s.ur,
			s.fingerprint,
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

		case req := <-s.sourceStaticImplSetReady:
			s.parent.onSourceStaticSetReady(s.ctx, req)

		case req := <-s.sourceStaticImplSetNotReady:
			s.parent.onSourceStaticSetNotReady(s.ctx, req)

		case <-s.ctx.Done():
			innerCtxCancel()
			<-implErr
			return
		}
	}
}

// onSourceAPIDescribe implements source.
func (s *sourceStatic) onSourceAPIDescribe() interface{} {
	return s.impl.onSourceAPIDescribe()
}

// onSourceStaticImplSetReady is called by a sourceStaticImpl.
func (s *sourceStatic) onSourceStaticImplSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes {
	req.res = make(chan pathSourceStaticSetReadyRes)
	select {
	case s.sourceStaticImplSetReady <- req:
		return <-req.res
	case <-s.ctx.Done():
		return pathSourceStaticSetReadyRes{err: fmt.Errorf("terminated")}
	}
}

// onSourceStaticImplSetNotReady is called by a sourceStaticImpl.
func (s *sourceStatic) onSourceStaticImplSetNotReady(req pathSourceStaticSetNotReadyReq) {
	req.res = make(chan struct{})
	select {
	case s.sourceStaticImplSetNotReady <- req:
		<-req.res
	case <-s.ctx.Done():
	}
}
