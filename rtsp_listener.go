package main

import (
	"log"
	"net"
	"sync"
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

func (l *rtspListener) run(wg sync.WaitGroup) {
	defer wg.Done()

	for {
		nconn, err := l.netl.AcceptTCP()
		if err != nil {
			break
		}

		rsc := newRtspClient(l.p, nconn)
		wg.Add(1)
		go rsc.run(wg)
	}
}
