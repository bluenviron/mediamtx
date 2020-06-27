package main

import (
	"log"
	"net"
)

type serverTcpListener struct {
	p     *program
	nconn *net.TCPListener

	done chan struct{}
}

func newServerTcpListener(p *program) (*serverTcpListener, error) {
	nconn, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: p.args.rtspPort,
	})
	if err != nil {
		return nil, err
	}

	l := &serverTcpListener{
		p:     p,
		nconn: nconn,
		done:  make(chan struct{}),
	}

	l.log("opened on :%d", p.args.rtspPort)
	return l, nil
}

func (l *serverTcpListener) log(format string, args ...interface{}) {
	log.Printf("[TCP listener] "+format, args...)
}

func (l *serverTcpListener) run() {
	for {
		nconn, err := l.nconn.AcceptTCP()
		if err != nil {
			break
		}

		l.p.events <- programEventClientNew{nconn}
	}

	close(l.done)
}

func (l *serverTcpListener) close() {
	l.nconn.Close()
	<-l.done
}
