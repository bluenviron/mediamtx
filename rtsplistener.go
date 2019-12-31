package main

import (
	"log"
	"net"
)

type rtspListener struct {
	p    *program
	netl *net.TCPListener
}

func newRtspListener(p *program) (*rtspListener, error) {
	netl, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: p.rtspPort,
	})
	if err != nil {
		return nil, err
	}

	s := &rtspListener{
		p:    p,
		netl: netl,
	}

	s.log("opened on :%d", p.rtspPort)
	return s, nil
}

func (l *rtspListener) log(format string, args ...interface{}) {
	log.Printf("[RTSP listener] "+format, args...)
}

func (l *rtspListener) run() {
	for {
		nconn, err := l.netl.AcceptTCP()
		if err != nil {
			break
		}

		rsc := newClient(l.p, nconn)
		go rsc.run()
	}
}
