package serverrtmp

import (
	"net"
	"strconv"
	"sync"

	"github.com/notedit/rtmp/format/rtmp"

	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/rtmputils"
)

// Parent is implemented by program.
type Parent interface {
	Log(logger.Level, string, ...interface{})
}

// Server is a RTMP listener.
type Server struct {
	l   net.Listener
	srv *rtmp.Server
	wg  sync.WaitGroup

	accept chan *rtmputils.Conn
}

// New allocates a Server.
func New(
	listenIP string,
	port int,
	parent Parent) (*Server, error) {

	address := listenIP + ":" + strconv.FormatInt(int64(port), 10)
	l, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	s := &Server{
		l:      l,
		accept: make(chan *rtmputils.Conn),
	}

	s.srv = rtmp.NewServer()
	s.srv.HandleConn = s.innerHandleConn

	parent.Log(logger.Info, "[RTMP listener] opened on %s", address)

	s.wg.Add(1)
	go s.run()

	return s, nil
}

// Close closes a Server.
func (s *Server) Close() {
	go func() {
		for co := range s.accept {
			co.NetConn().Close()
		}
	}()
	s.l.Close()
	s.wg.Wait()
	close(s.accept)
}

func (s *Server) run() {
	defer s.wg.Done()

	for {
		nconn, err := s.l.Accept()
		if err != nil {
			break
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.srv.HandleNetConn(nconn)
		}()
	}
}

func (s *Server) innerHandleConn(rconn *rtmp.Conn, nconn net.Conn) {
	s.accept <- rtmputils.NewConn(rconn, nconn)
}

// Accept returns a channel to accept incoming connections.
func (s *Server) Accept() chan *rtmputils.Conn {
	return s.accept
}
