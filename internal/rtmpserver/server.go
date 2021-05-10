package rtmpserver

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/pathman"
	"github.com/aler9/rtsp-simple-server/internal/rtmpconn"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

// Parent is implemented by program.
type Parent interface {
	Log(logger.Level, string, ...interface{})
}

// Server is a RTMP server.
type Server struct {
	readTimeout         time.Duration
	writeTimeout        time.Duration
	readBufferCount     int
	rtspAddress         string
	runOnConnect        string
	runOnConnectRestart bool
	stats               *stats.Stats
	pathMan             *pathman.PathManager
	parent              Parent

	ctx       context.Context
	ctxCancel func()
	wg        sync.WaitGroup
	l         net.Listener
	conns     map[*rtmpconn.Conn]struct{}

	// in
	connClose chan *rtmpconn.Conn
}

// New allocates a Server.
func New(
	address string,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount int,
	rtspAddress string,
	runOnConnect string,
	runOnConnectRestart bool,
	stats *stats.Stats,
	pathMan *pathman.PathManager,
	parent Parent) (*Server, error) {

	l, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	s := &Server{
		readTimeout:         readTimeout,
		writeTimeout:        writeTimeout,
		readBufferCount:     readBufferCount,
		rtspAddress:         rtspAddress,
		runOnConnect:        runOnConnect,
		runOnConnectRestart: runOnConnectRestart,
		stats:               stats,
		pathMan:             pathMan,
		parent:              parent,
		ctx:                 ctx,
		ctxCancel:           ctxCancel,
		l:                   l,
		conns:               make(map[*rtmpconn.Conn]struct{}),
		connClose:           make(chan *rtmpconn.Conn),
	}

	s.Log(logger.Info, "listener opened on %s", address)

	s.wg.Add(1)
	go s.run()

	return s, nil
}

// Log is the main logging function.
func (s *Server) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[RTMP] "+format, append([]interface{}{}, args...)...)
}

// Close closes a Server.
func (s *Server) Close() {
	s.ctxCancel()
	s.wg.Wait()
}

func (s *Server) run() {
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
			c := rtmpconn.New(
				s.rtspAddress,
				s.readTimeout,
				s.writeTimeout,
				s.readBufferCount,
				s.runOnConnect,
				s.runOnConnectRestart,
				&s.wg,
				s.stats,
				nconn,
				s.pathMan,
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

func (s *Server) doConnClose(c *rtmpconn.Conn) {
	delete(s.conns, c)
	c.ParentClose()
	c.Close()
}

// OnConnClose is called by rtmpconn.Conn.
func (s *Server) OnConnClose(c *rtmpconn.Conn) {
	select {
	case s.connClose <- c:
	case <-s.ctx.Done():
	}
}
