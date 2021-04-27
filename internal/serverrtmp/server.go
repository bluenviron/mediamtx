package serverrtmp

import (
	"net"
	"sync"
	"time"

	"github.com/aler9/rtsp-simple-server/internal/clientrtmp"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/pathman"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

// Parent is implemented by program.
type Parent interface {
	Log(logger.Level, string, ...interface{})
}

// Server is a RTMP listener.
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

	l       net.Listener
	wg      sync.WaitGroup
	clients map[*clientrtmp.Client]struct{}

	// in
	clientClose chan *clientrtmp.Client
	terminate   chan struct{}

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
		clients:             make(map[*clientrtmp.Client]struct{}),
		clientClose:         make(chan *clientrtmp.Client),
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
			c := clientrtmp.New(
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
			s.clients[c] = struct{}{}

		case c := <-s.clientClose:
			if _, ok := s.clients[c]; !ok {
				continue
			}
			s.doClientClose(c)

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

			case _, ok := <-s.clientClose:
				if !ok {
					return
				}
			}
		}
	}()

	s.l.Close()

	for c := range s.clients {
		s.doClientClose(c)
	}

	s.wg.Wait()

	close(acceptErr)
	close(connNew)
	close(s.clientClose)
}

func (s *Server) doClientClose(c *clientrtmp.Client) {
	delete(s.clients, c)
	c.Close()
}

// OnClientClose is called by clientrtmp.Client.
func (s *Server) OnClientClose(c *clientrtmp.Client) {
	s.clientClose <- c
}
