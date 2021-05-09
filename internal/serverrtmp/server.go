package serverrtmp

import (
	"net"
	"sync"
	"time"

	"github.com/aler9/rtsp-simple-server/internal/connrtmp"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/pathman"
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

	l     net.Listener
	wg    sync.WaitGroup
	conns map[*connrtmp.Conn]struct{}

	// in
	connClose chan *connrtmp.Conn
	terminate chan struct{}

	// out
	done chan struct{}
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
		l:                   l,
		conns:               make(map[*connrtmp.Conn]struct{}),
		connClose:           make(chan *connrtmp.Conn),
		terminate:           make(chan struct{}),
		done:                make(chan struct{}),
	}

	s.Log(logger.Info, "listener opened on %s", address)

	go s.run()

	return s, nil
}

// Log is the main logging function.
func (s *Server) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[RTMP] "+format, append([]interface{}{}, args...)...)
}

// Close closes a Server.
func (s *Server) Close() {
	close(s.terminate)
	<-s.done
}

func (s *Server) run() {
	defer close(s.done)

	s.wg.Add(1)
	connNew := make(chan net.Conn)
	acceptErr := make(chan error)
	go func() {
		defer s.wg.Done()
		acceptErr <- func() error {
			for {
				conn, err := s.l.Accept()
				if err != nil {
					return err
				}

				connNew <- conn
			}
		}()
	}()

outer:
	for {
		select {
		case err := <-acceptErr:
			s.Log(logger.Warn, "ERR: %s", err)
			break outer

		case nconn := <-connNew:
			c := connrtmp.New(
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

		case <-s.terminate:
			break outer
		}
	}

	go func() {
		for {
			select {
			case _, ok := <-acceptErr:
				if !ok {
					return
				}

			case conn, ok := <-connNew:
				if !ok {
					return
				}
				conn.Close()

			case _, ok := <-s.connClose:
				if !ok {
					return
				}
			}
		}
	}()

	s.l.Close()

	for c := range s.conns {
		s.doConnClose(c)
	}

	s.wg.Wait()

	close(acceptErr)
	close(connNew)
	close(s.connClose)
}

func (s *Server) doConnClose(c *connrtmp.Conn) {
	delete(s.conns, c)
	c.ParentClose()
	c.Close()
}

// OnConnClose is called by connrtmp.Conn.
func (s *Server) OnConnClose(c *connrtmp.Conn) {
	s.connClose <- c
}
