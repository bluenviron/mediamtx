package serverrtmp

import (
	"net"
	"strconv"
	"sync/atomic"

	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/rtmp"
)

// Parent is implemented by program.
type Parent interface {
	Log(logger.Level, string, ...interface{})
}

// Server is a RTMP listener.
type Server struct {
	parent Parent

	l      net.Listener
	closed uint32

	// out
	accept chan *rtmp.Conn
	done   chan struct{}
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
		parent: parent,
		l:      l,
		accept: make(chan *rtmp.Conn),
		done:   make(chan struct{}),
	}

	s.log(logger.Info, "opened on %s", address)

	go s.run()

	return s, nil
}

func (s *Server) log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[RTMP listener] "+format, append([]interface{}{}, args...)...)
}

// Close closes a Server.
func (s *Server) Close() {
	go func() {
		for co := range s.accept {
			co.NetConn().Close()
		}
	}()
	atomic.StoreUint32(&s.closed, 1)
	s.l.Close()
	<-s.done
}

func (s *Server) run() {
	defer close(s.done)

	for {
		nconn, err := s.l.Accept()
		if err != nil {
			if atomic.LoadUint32(&s.closed) == 1 {
				break
			}
			s.log(logger.Warn, "ERR: %s", err)
			continue
		}

		s.accept <- rtmp.NewServerConn(nconn)
	}

	close(s.accept)
}

// Accept returns a channel to accept incoming connections.
func (s *Server) Accept() chan *rtmp.Conn {
	return s.accept
}
