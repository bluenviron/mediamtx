package servertcp

import (
	"net"
)

type Parent interface {
	Log(string, ...interface{})
}

type Server struct {
	parent Parent

	listener *net.TCPListener

	// out
	accept chan net.Conn
	done   chan struct{}
}

func New(port int, parent Parent) (*Server, error) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: port,
	})
	if err != nil {
		return nil, err
	}

	s := &Server{
		parent:   parent,
		listener: listener,
		accept:   make(chan net.Conn),
		done:     make(chan struct{}),
	}

	parent.Log("[TCP server] opened on :%d", port)

	go s.run()
	return s, nil
}

func (s *Server) Close() {
	go func() {
		for co := range s.accept {
			co.Close()
		}
	}()
	s.listener.Close()
	<-s.done
}

func (s *Server) run() {
	defer close(s.done)

	for {
		conn, err := s.listener.AcceptTCP()
		if err != nil {
			break
		}

		s.accept <- conn
	}

	close(s.accept)
}

func (s *Server) Accept() <-chan net.Conn {
	return s.accept
}
