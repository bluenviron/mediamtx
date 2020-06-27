package main

import (
	"net"
	"time"
)

type udpWrite struct {
	addr *net.UDPAddr
	buf  []byte
}

type serverUdpListener struct {
	p             *program
	nconn         *net.UDPConn
	trackFlowType trackFlowType

	write chan *udpWrite
	done  chan struct{}
}

func newServerUdpListener(p *program, port int, trackFlowType trackFlowType) (*serverUdpListener, error) {
	nconn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: port,
	})
	if err != nil {
		return nil, err
	}

	l := &serverUdpListener{
		p:             p,
		nconn:         nconn,
		trackFlowType: trackFlowType,
		write:         make(chan *udpWrite),
		done:          make(chan struct{}),
	}

	l.log("opened on :%d", port)
	return l, nil
}

func (l *serverUdpListener) log(format string, args ...interface{}) {
	var label string
	if l.trackFlowType == _TRACK_FLOW_RTP {
		label = "RTP"
	} else {
		label = "RTCP"
	}
	l.p.log("[UDP/"+label+" listener] "+format, args...)
}

func (l *serverUdpListener) run() {
	go func() {
		for w := range l.write {
			l.nconn.SetWriteDeadline(time.Now().Add(l.p.args.writeTimeout))
			l.nconn.WriteTo(w.buf, w.addr)
		}
	}()

	for {
		// create a buffer for each read.
		// this is necessary since the buffer is propagated with channels
		// so it must be unique.
		buf := make([]byte, 2048) // UDP MTU is 1400
		n, addr, err := l.nconn.ReadFromUDP(buf)
		if err != nil {
			break
		}

		l.p.events <- programEventFrameUdp{
			l.trackFlowType,
			addr,
			buf[:n],
		}
	}

	close(l.write)

	close(l.done)
}

func (l *serverUdpListener) close() {
	l.nconn.Close()
	<-l.done
}
