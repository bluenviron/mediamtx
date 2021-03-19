package serverrtsp

import (
	"crypto/tls"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

// Parent is implemented by program.
type Parent interface {
	Log(logger.Level, string, ...interface{})
}

// Server is a RTSP listener.
type Server struct {
	useTLS bool
	parent Parent

	srv    *gortsplib.Server
	closed uint32

	// out
	accept chan *gortsplib.ServerConn
	done   chan struct{}
}

// New allocates a Server.
func New(
	listenIP string,
	port int,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount int,
	readBufferSize int,
	useUDP bool,
	rtpPort int,
	rtcpPort int,
	useTLS bool,
	serverCert string,
	serverKey string,
	parent Parent) (*Server, error) {

	conf := gortsplib.ServerConf{
		ReadTimeout:     readTimeout,
		WriteTimeout:    writeTimeout,
		ReadBufferCount: readBufferCount,
		ReadBufferSize:  readBufferSize,
	}

	if useUDP {
		conf.UDPRTPAddress = listenIP + ":" + strconv.FormatInt(int64(rtpPort), 10)
		conf.UDPRTCPAddress = listenIP + ":" + strconv.FormatInt(int64(rtcpPort), 10)
	}

	if useTLS {
		cert, err := tls.LoadX509KeyPair(serverCert, serverKey)
		if err != nil {
			return nil, err
		}

		conf.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	}

	address := listenIP + ":" + strconv.FormatInt(int64(port), 10)
	srv, err := conf.Serve(address)
	if err != nil {
		return nil, err
	}

	s := &Server{
		useTLS: useTLS,
		parent: parent,
		srv:    srv,
		accept: make(chan *gortsplib.ServerConn),
		done:   make(chan struct{}),
	}

	if conf.UDPRTPAddress != "" {
		parent.Log(logger.Info, "[RTSP/UDP/RTP listener] opened on %s", conf.UDPRTPAddress)
	}

	if conf.UDPRTCPAddress != "" {
		parent.Log(logger.Info, "[RTSP/UDP/RTCP listener] opened on %s", conf.UDPRTCPAddress)
	}

	s.log(logger.Info, "opened on %s", address)

	go s.run()

	return s, nil
}

func (s *Server) log(level logger.Level, format string, args ...interface{}) {
	label := func() string {
		if s.useTLS {
			return "RTSP/TLS"
		}
		return "RTSP/TCP"
	}()
	s.parent.Log(level, "[%s listener] "+format, append([]interface{}{label}, args...)...)
}

// Close closes a Server.
func (s *Server) Close() {
	go func() {
		for co := range s.accept {
			co.Close()
		}
	}()
	atomic.StoreUint32(&s.closed, 1)
	s.srv.Close()
	<-s.done
}

func (s *Server) run() {
	defer close(s.done)

	for {
		conn, err := s.srv.Accept()
		if err != nil {
			if atomic.LoadUint32(&s.closed) == 1 {
				break
			}
			s.log(logger.Warn, "ERR: %s", err)
			continue
		}

		s.accept <- conn
	}

	close(s.accept)
}

// Accept returns a channel to accept incoming connections.
func (s *Server) Accept() chan *gortsplib.ServerConn {
	return s.accept
}
