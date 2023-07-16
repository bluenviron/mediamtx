package core

import (
	"context"
	"sync"
	"time"

	"github.com/datarhei/gosrt"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
)

func srtMaxPayloadSize(u int) int {
	return u - 16 // SRT header size
}

type srtNewConnReq struct {
	connReq srt.ConnRequest
	res     chan *srtConn
}

type srtServerParent interface {
	logger.Writer
}

type srtServer struct {
	readTimeout       conf.StringDuration
	writeTimeout      conf.StringDuration
	readBufferCount   int
	udpMaxPayloadSize int
	pathManager       *pathManager
	parent            srtServerParent

	ctx       context.Context
	ctxCancel func()
	wg        sync.WaitGroup
	ln        srt.Listener
	conns     map[*srtConn]struct{}

	// in
	chNewConnRequest chan srtNewConnReq
	chAcceptErr      chan error
	chCloseConn      chan *srtConn
}

func newSRTServer(
	address string,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
	udpMaxPayloadSize int,
	pathManager *pathManager,
	parent srtServerParent,
) (*srtServer, error) {
	conf := srt.DefaultConfig()
	conf.ConnectionTimeout = time.Duration(readTimeout)
	conf.PayloadSize = uint32(srtMaxPayloadSize(udpMaxPayloadSize))

	ln, err := srt.Listen("srt", address, conf)
	if err != nil {
		return nil, err
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	s := &srtServer{
		readTimeout:       readTimeout,
		writeTimeout:      writeTimeout,
		readBufferCount:   readBufferCount,
		udpMaxPayloadSize: udpMaxPayloadSize,
		pathManager:       pathManager,
		parent:            parent,
		ctx:               ctx,
		ctxCancel:         ctxCancel,
		ln:                ln,
		conns:             make(map[*srtConn]struct{}),
		chNewConnRequest:  make(chan srtNewConnReq),
		chAcceptErr:       make(chan error),
		chCloseConn:       make(chan *srtConn),
	}

	s.Log(logger.Info, "listener opened on "+address+" (UDP)")

	newSRTListener(
		s.ln,
		&s.wg,
		s,
	)

	s.wg.Add(1)
	go s.run()

	return s, nil
}

// Log is the main logging function.
func (s *srtServer) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[SRT] "+format, append([]interface{}{}, args...)...)
}

func (s *srtServer) close() {
	s.ctxCancel()
	s.wg.Wait()
}

func (s *srtServer) run() {
	defer s.wg.Done()

outer:
	for {
		select {
		case err := <-s.chAcceptErr:
			s.Log(logger.Error, "%s", err)
			break outer

		case req := <-s.chNewConnRequest:
			c := newSRTConn(
				s.ctx,
				s.readTimeout,
				s.writeTimeout,
				s.readBufferCount,
				s.udpMaxPayloadSize,
				req.connReq,
				&s.wg,
				s.pathManager,
				s)
			s.conns[c] = struct{}{}
			req.res <- c

		case c := <-s.chCloseConn:
			delete(s.conns, c)

		case <-s.ctx.Done():
			break outer
		}
	}

	s.ctxCancel()

	s.ln.Close()
}

// newConnRequest is called by srtListener.
func (s *srtServer) newConnRequest(connReq srt.ConnRequest) *srtConn {
	req := srtNewConnReq{
		connReq: connReq,
		res:     make(chan *srtConn),
	}

	select {
	case s.chNewConnRequest <- req:
		c := <-req.res

		return c.new(req)

	case <-s.ctx.Done():
		return nil
	}
}

// acceptError is called by srtListener.
func (s *srtServer) acceptError(err error) {
	select {
	case s.chAcceptErr <- err:
	case <-s.ctx.Done():
	}
}

// closeConn is called by srtConn.
func (s *srtServer) closeConn(c *srtConn) {
	select {
	case s.chCloseConn <- c:
	case <-s.ctx.Done():
	}
}
