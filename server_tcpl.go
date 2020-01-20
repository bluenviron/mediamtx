package main

import (
	"log"
	"net"
)

type serverTcpListener struct {
	p    *program
	netl *net.TCPListener
}

func newServerTcpListener(p *program) (*serverTcpListener, error) {
	netl, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: p.rtspPort,
	})
	if err != nil {
		return nil, err
	}

	s := &serverTcpListener{
		p:    p,
		netl: netl,
	}

	s.log("opened on :%d", p.rtspPort)
	return s, nil
}

func (l *serverTcpListener) log(format string, args ...interface{}) {
	log.Printf("[TCP listener] "+format, args...)
}

func (l *serverTcpListener) run() {
	for {
		nconn, err := l.netl.AcceptTCP()
		if err != nil {
			break
		}

		rsc := newClient(l.p, nconn)
		go rsc.run()
	}
}
