package main

import (
	"log"
	"net"
	"sync"
)

type udpListener struct {
	nconn     *net.UDPConn
	logPrefix string
	cb        func([]byte)
}

func newUdpListener(port int, logPrefix string, cb func([]byte)) (*udpListener, error) {
	nconn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: port,
	})
	if err != nil {
		return nil, err
	}

	l := &udpListener{
		nconn:     nconn,
		logPrefix: logPrefix,
		cb:        cb,
	}

	l.log("opened on :%d", port)
	return l, nil
}

func (l *udpListener) log(format string, args ...interface{}) {
	log.Printf("["+l.logPrefix+" listener] "+format, args...)
}

func (l *udpListener) run(wg sync.WaitGroup) {
	defer wg.Done()

	buf := make([]byte, 2048) // UDP MTU is 1400

	for {
		n, _, err := l.nconn.ReadFromUDP(buf)
		if err != nil {
			l.log("ERR: %s", err)
			break
		}

		l.cb(buf[:n])
	}
}
