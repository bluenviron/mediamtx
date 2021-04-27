package serverrtsp

import (
	"crypto/tls"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"

	"github.com/aler9/rtsp-simple-server/internal/clientrtsp"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/pathman"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

// Parent is implemented by program.
type Parent interface {
	Log(logger.Level, string, ...interface{})
}

// Server is a RTSP listener.
type Server struct {
	readTimeout         time.Duration
	isTLS               bool
	rtspAddress         string
	protocols           map[base.StreamProtocol]struct{}
	runOnConnect        string
	runOnConnectRestart bool
	stats               *stats.Stats
	pathMan             *pathman.PathManager
	parent              Parent

	srv     *gortsplib.Server
	wg      sync.WaitGroup
	clients map[*clientrtsp.Client]struct{}

	// in
	clientClose chan *clientrtsp.Client
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
	readBufferSize int,
	useUDP bool,
	rtpAddress string,
	rtcpAddress string,
	isTLS bool,
	serverCert string,
	serverKey string,
	rtspAddress string,
	protocols map[base.StreamProtocol]struct{},
	runOnConnect string,
	runOnConnectRestart bool,
	stats *stats.Stats,
	pathMan *pathman.PathManager,
	parent Parent) (*Server, error) {

	conf := gortsplib.ServerConf{
		ReadTimeout:     readTimeout,
		WriteTimeout:    writeTimeout,
		ReadBufferCount: readBufferCount,
		ReadBufferSize:  readBufferSize,
	}

	if useUDP {
		conf.UDPRTPAddress = rtpAddress
		conf.UDPRTCPAddress = rtcpAddress
	}

	if isTLS {
		cert, err := tls.LoadX509KeyPair(serverCert, serverKey)
		if err != nil {
			return nil, err
		}

		conf.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	}

	srv, err := conf.Serve(address)
	if err != nil {
		return nil, err
	}

	s := &Server{
		readTimeout: readTimeout,
		isTLS:       isTLS,
		rtspAddress: rtspAddress,
		protocols:   protocols,
		stats:       stats,
		pathMan:     pathMan,
		parent:      parent,
		srv:         srv,
		clients:     make(map[*clientrtsp.Client]struct{}),
		clientClose: make(chan *clientrtsp.Client),
		terminate:   make(chan struct{}),
		done:        make(chan struct{}),
	}

	if conf.UDPRTPAddress != "" {
		s.Log(logger.Info, "UDP/RTP listener opened on %s", conf.UDPRTPAddress)
	}

	if conf.UDPRTCPAddress != "" {
		s.Log(logger.Info, "UDP/RTCP listener opened on %s", conf.UDPRTCPAddress)
	}

	s.Log(logger.Info, "TCP listener opened on %s", address)

	go s.run()

	return s, nil
}

// Log is the main logging function.
func (s *Server) Log(level logger.Level, format string, args ...interface{}) {
	label := func() string {
		if s.isTLS {
			return "RTSPS"
		}
		return "RTSP"
	}()
	s.parent.Log(level, "[%s] "+format, append([]interface{}{label}, args...)...)
}

// Close closes a Server.
func (s *Server) Close() {
	close(s.terminate)
	<-s.done
}

func (s *Server) run() {
	defer close(s.done)

	s.wg.Add(1)
	connNew := make(chan *gortsplib.ServerConn)
	acceptErr := make(chan error)
	go func() {
		defer s.wg.Done()
		acceptErr <- func() error {
			for {
				conn, err := s.srv.Accept()
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

		case conn := <-connNew:
			c := clientrtsp.New(
				s.isTLS,
				s.rtspAddress,
				s.readTimeout,
				s.runOnConnect,
				s.runOnConnectRestart,
				s.protocols,
				&s.wg,
				s.stats,
				conn,
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

	s.srv.Close()

	for c := range s.clients {
		s.doClientClose(c)
	}

	s.wg.Wait()

	close(acceptErr)
	close(connNew)
	close(s.clientClose)
}

func (s *Server) doClientClose(c *clientrtsp.Client) {
	delete(s.clients, c)
	c.Close()
}

// OnClientClose is called by a client.
func (s *Server) OnClientClose(c *clientrtsp.Client) {
	s.clientClose <- c
}
