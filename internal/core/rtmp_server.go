package core

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type rtmpServerParent interface {
	Log(logger.Level, string, ...interface{})
}

type rtmpServer struct {
	readTimeout         time.Duration
	writeTimeout        time.Duration
	readBufferCount     int
	rtspAddress         string
	runOnConnect        string
	runOnConnectRestart bool
	stats               *stats
	pathManager         *pathManager
	parent              rtmpServerParent

	ctx       context.Context
	ctxCancel func()
	wg        sync.WaitGroup
	l         net.Listener
	conns     map[*rtmpConn]struct{}

	// in
	connClose chan *rtmpConn
}

func newRTMPServer(
	parentCtx context.Context,
	address string,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount int,
	rtspAddress string,
	runOnConnect string,
	runOnConnectRestart bool,
	stats *stats,
	pathManager *pathManager,
	parent rtmpServerParent) (*rtmpServer, error) {
	l, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &rtmpServer{
		readTimeout:         readTimeout,
		writeTimeout:        writeTimeout,
		readBufferCount:     readBufferCount,
		rtspAddress:         rtspAddress,
		runOnConnect:        runOnConnect,
		runOnConnectRestart: runOnConnectRestart,
		stats:               stats,
		pathManager:         pathManager,
		parent:              parent,
		ctx:                 ctx,
		ctxCancel:           ctxCancel,
		l:                   l,
		conns:               make(map[*rtmpConn]struct{}),
		connClose:           make(chan *rtmpConn),
	}

	s.Log(logger.Info, "listener opened on %s", address)

	s.wg.Add(1)
	go s.run()

	return s, nil
}

func (s *rtmpServer) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[RTMP] "+format, append([]interface{}{}, args...)...)
}

func (s *rtmpServer) close() {
	s.ctxCancel()
	s.wg.Wait()
}

func (s *rtmpServer) run() {
	defer s.wg.Done()

	s.wg.Add(1)
	connNew := make(chan net.Conn)
	acceptErr := make(chan error)
	go func() {
		defer s.wg.Done()
		err := func() error {
			for {
				conn, err := s.l.Accept()
				if err != nil {
					return err
				}

				select {
				case connNew <- conn:
				case <-s.ctx.Done():
					conn.Close()
				}
			}
		}()

		select {
		case acceptErr <- err:
		case <-s.ctx.Done():
		}
	}()

outer:
	for {
		select {
		case err := <-acceptErr:
			s.Log(logger.Warn, "ERR: %s", err)
			break outer

		case nconn := <-connNew:
			c := newRTMPConn(
				s.ctx,
				s.rtspAddress,
				s.readTimeout,
				s.writeTimeout,
				s.readBufferCount,
				s.runOnConnect,
				s.runOnConnectRestart,
				&s.wg,
				s.stats,
				nconn,
				s.pathManager,
				s)
			s.conns[c] = struct{}{}

		case c := <-s.connClose:
			if _, ok := s.conns[c]; !ok {
				continue
			}
			s.doConnClose(c)

		case <-s.ctx.Done():
			break outer
		}
	}

	s.ctxCancel()

	s.l.Close()

	for c := range s.conns {
		s.doConnClose(c)
	}
}

func (s *rtmpServer) doConnClose(c *rtmpConn) {
	delete(s.conns, c)
	c.ParentClose()
	c.Close()
}

// OnConnClose is called by rtmpConn.
func (s *rtmpServer) OnConnClose(c *rtmpConn) {
	select {
	case s.connClose <- c:
	case <-s.ctx.Done():
	}
}
