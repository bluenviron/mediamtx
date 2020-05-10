package main

import (
	"log"
	"net"
	"sync"
)

type serverTcpListener struct {
	p          *program
	netl       *net.TCPListener
	mutex      sync.RWMutex
	clients    map[*serverClient]struct{}
	publishers map[string]*serverClient
}

func newServerTcpListener(p *program) (*serverTcpListener, error) {
	netl, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: p.args.rtspPort,
	})
	if err != nil {
		return nil, err
	}

	s := &serverTcpListener{
		p:          p,
		netl:       netl,
		clients:    make(map[*serverClient]struct{}),
		publishers: make(map[string]*serverClient),
	}

	s.log("opened on :%d", p.args.rtspPort)
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

		rsc := newServerClient(l.p, nconn)
		go rsc.run()
	}
}
