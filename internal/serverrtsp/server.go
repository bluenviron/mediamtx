package serverrtsp

import (
	"crypto/tls"
	"strconv"
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
	parent Parent

	srv *gortsplib.Server

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

	label := func() string {
		if conf.TLSConfig != nil {
			return "RTSP/TLS"
		}
		return "RTSP/TCP"
	}()
	parent.Log(logger.Info, "[%s listener] opened on %s", label, address)

	go s.run()

	return s, nil
}

// Close closes a Server.
func (s *Server) Close() {
	go func() {
		for co := range s.accept {
			co.Close()
		}
	}()

	s.srv.Close()
	<-s.done
}

func (s *Server) run() {
	defer close(s.done)

	for {
		conn, err := s.srv.Accept()
		if err != nil {
			break
		}

		s.accept <- conn
	}

	close(s.accept)
}

// Accept returns a channel to accept incoming connections.
func (s *Server) Accept() chan *gortsplib.ServerConn {
	return s.accept
}
