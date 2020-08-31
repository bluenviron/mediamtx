package main

import (
	"net"
)

type serverTcp struct {
	p        *program
	listener *net.TCPListener

	done chan struct{}
}

func newServerTcp(p *program) (*serverTcp, error) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: p.conf.RtspPort,
	})
	if err != nil {
		return nil, err
	}

	l := &serverTcp{
		p:        p,
		listener: listener,
		done:     make(chan struct{}),
	}

	l.log("opened on :%d", p.conf.RtspPort)
	return l, nil
}

func (l *serverTcp) log(format string, args ...interface{}) {
	l.p.log("[TCP listener] "+format, args...)
}

func (l *serverTcp) run() {
	for {
		conn, err := l.listener.AcceptTCP()
		if err != nil {
			break
		}

		l.p.clientNew <- conn
	}

	close(l.done)
}

func (l *serverTcp) close() {
	l.listener.Close()
	<-l.done
}
