package servertls

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

// Server is a TCP/TLS/RTSPS listener.
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
	readBufferCount uint64,
	serverKey string,
	serverCert string,
	parent Parent) (*Server, error) {

	cert, err := tls.LoadX509KeyPair(serverCert, serverKey)
	if err != nil {
		return nil, err
	}

	conf := gortsplib.ServerConf{
		TLSConfig:       &tls.Config{Certificates: []tls.Certificate{cert}},
		ReadTimeout:     readTimeout,
		WriteTimeout:    writeTimeout,
		ReadBufferCount: readBufferCount,
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

	parent.Log(logger.Info, "[TCP/TLS/RTSPS listener] opened on %s", address)

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
