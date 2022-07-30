package core

import (
	"context"
	"strings"
	"sync"
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
	onSourceStaticSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	onSourceStaticSetNotReady(req pathSourceStaticSetNotReadyReq)
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
	wg              *sync.WaitGroup
	parent          sourceStaticParent

	impl      sourceStaticImpl
	ctx       context.Context
	ctxCancel func()
}

func newSourceStatic(
	parentCtx context.Context,
	ur string,
	protocol conf.SourceProtocol,
	anyPortEnable bool,
	fingerprint string,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
	wg *sync.WaitGroup,
	parent sourceStaticParent,
) *sourceStatic {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &sourceStatic{
		ur:              ur,
		protocol:        protocol,
		anyPortEnable:   anyPortEnable,
		fingerprint:     fingerprint,
		readTimeout:     readTimeout,
		writeTimeout:    writeTimeout,
		readBufferCount: readBufferCount,
		wg:              wg,
		parent:          parent,
		ctx:             ctx,
		ctxCancel:       ctxCancel,
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

	s.impl.Log(logger.Info, "started")

	s.wg.Add(1)
	go s.run()

	return s
}

func (s *sourceStatic) close() {
	s.impl.Log(logger.Info, "stopped")
	s.ctxCancel()
}

func (s *sourceStatic) log(level logger.Level, format string, args ...interface{}) {
	s.parent.log(level, format, args...)
}

func (s *sourceStatic) run() {
	defer s.wg.Done()

outer:
	for {
		innerCtx, innerCtxCancel := context.WithCancel(context.Background())
		innerErr := make(chan error)
		go func() {
			innerErr <- s.impl.run(innerCtx)
		}()

		select {
		case err := <-innerErr:
			innerCtxCancel()
			s.impl.Log(logger.Info, "ERR: %v", err)

		case <-s.ctx.Done():
			innerCtxCancel()
			<-innerErr
		}

		select {
		case <-time.After(sourceStaticRetryPause):
		case <-s.ctx.Done():
			break outer
		}
	}

	s.ctxCancel()
}

// onSourceAPIDescribe implements source.
func (s *sourceStatic) onSourceAPIDescribe() interface{} {
	return s.impl.onSourceAPIDescribe()
}

// onSourceStaticSetReady is called by a sourceStaticImpl.
func (s *sourceStatic) onSourceStaticSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes {
	req.source = s
	return s.parent.onSourceStaticSetReady(req)
}

// onSourceStaticSetNotReady is called by a sourceStaticImpl.
func (s *sourceStatic) onSourceStaticSetNotReady(req pathSourceStaticSetNotReadyReq) {
	req.source = s
	s.parent.onSourceStaticSetNotReady(req)
}
