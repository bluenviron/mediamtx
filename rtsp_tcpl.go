package main

import (
	"log"
	"net"
)

type rtspTcpListener struct {
	p    *program
	netl *net.TCPListener
}

func newRtspTcpListener(p *program) (*rtspTcpListener, error) {
	netl, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: p.rtspPort,
	})
	if err != nil {
		return nil, err
	}

	s := &rtspTcpListener{
		p:    p,
		netl: netl,
	}

	s.log("opened on :%d", p.rtspPort)
	return s, nil
}

func (l *rtspTcpListener) log(format string, args ...interface{}) {
	log.Printf("[RTSP TCP listener] "+format, args...)
}

func (l *rtspTcpListener) run() {
	for {
		nconn, err := l.netl.AcceptTCP()
		if err != nil {
			break
		}

		rsc := newClient(l.p, nconn)
		go rsc.run()
	}
}
