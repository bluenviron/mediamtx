package main

import (
	"log"
	"net"
	"sync"

	"rtsp-server/rtsp"
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
		conn, err := l.netl.AcceptTCP()
		if err != nil {
			break
		}

		rconn := rtsp.NewConn(conn)
		rsc := newRtspClient(l.p, rconn)
		wg.Add(1)
		go rsc.run(wg)
	}
}
