package main

import (
	"net"
)

type serverTcp struct {
	p     *program
	nconn *net.TCPListener

	done chan struct{}
}

func newServerTcp(p *program) (*serverTcp, error) {
	nconn, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: p.conf.RtspPort,
	})
	if err != nil {
		return nil, err
	}

	l := &serverTcp{
		p:     p,
		nconn: nconn,
		done:  make(chan struct{}),
	}

	l.log("opened on :%d", p.conf.RtspPort)
	return l, nil
}

func (l *serverTcp) log(format string, args ...interface{}) {
	l.p.log("[TCP listener] "+format, args...)
}

func (l *serverTcp) run() {
	for {
		nconn, err := l.nconn.AcceptTCP()
		if err != nil {
			break
		}

		l.p.events <- programEventClientNew{nconn}
	}

	close(l.done)
}

func (l *serverTcp) close() {
	l.nconn.Close()
	<-l.done
}
