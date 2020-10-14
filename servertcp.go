package main

import (
	"net"
)

type serverTCP struct {
	p        *program
	listener *net.TCPListener

	done chan struct{}
}

func newServerTCP(p *program) (*serverTCP, error) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: p.conf.RtspPort,
	})
	if err != nil {
		return nil, err
	}

	l := &serverTCP{
		p:        p,
		listener: listener,
		done:     make(chan struct{}),
	}

	l.log("opened on :%d", p.conf.RtspPort)
	return l, nil
}

func (l *serverTCP) log(format string, args ...interface{}) {
	l.p.log("[TCP server] "+format, args...)
}

func (l *serverTCP) run() {
	defer close(l.done)

	for {
		conn, err := l.listener.AcceptTCP()
		if err != nil {
			break
		}

		l.p.clientNew <- conn
	}
}

func (l *serverTCP) close() {
	l.listener.Close()
	<-l.done
}
