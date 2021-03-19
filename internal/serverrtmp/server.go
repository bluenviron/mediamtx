package serverrtmp

import (
	"net"
	"strconv"
	"sync"
	"sync/atomic"

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
	parent Parent

	l      net.Listener
	srv    *rtmp.Server
	closed uint32
	wg     sync.WaitGroup

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
		parent: parent,
		l:      l,
		accept: make(chan *rtmputils.Conn),
	}

	s.srv = rtmp.NewServer()
	s.srv.HandleConn = s.innerHandleConn

	s.log(logger.Info, "opened on %s", address)

	s.wg.Add(1)
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
	s.wg.Wait()
	close(s.accept)
}

func (s *Server) run() {
	defer s.wg.Done()

	for {
		nconn, err := s.l.Accept()
		if err != nil {
			if atomic.LoadUint32(&s.closed) == 1 {
				break
			}
			s.log(logger.Warn, "ERR: %s", err)
			continue
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
