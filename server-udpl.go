package main

import (
	"net"
	"time"
)

type udpAddrFramePair struct {
	addr *net.UDPAddr
	buf  []byte
}

type serverUdpListener struct {
	p             *program
	nconn         *net.UDPConn
	trackFlowType trackFlowType
	readBuf       *doubleBuffer
	writeBuf      *doubleBuffer

	writeChan chan *udpAddrFramePair
	done      chan struct{}
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
		readBuf:       newDoubleBuffer(2048),
		writeBuf:      newDoubleBuffer(2048),
		writeChan:     make(chan *udpAddrFramePair),
		done:          make(chan struct{}),
	}

	l.log("opened on :%d", port)
	return l, nil
}

func (l *serverUdpListener) log(format string, args ...interface{}) {
	var label string
	if l.trackFlowType == _TRACK_FLOW_TYPE_RTP {
		label = "RTP"
	} else {
		label = "RTCP"
	}
	l.p.log("[UDP/"+label+" listener] "+format, args...)
}

func (l *serverUdpListener) run() {
	go func() {
		for w := range l.writeChan {
			l.nconn.SetWriteDeadline(time.Now().Add(l.p.conf.WriteTimeout))
			l.nconn.WriteTo(w.buf, w.addr)
		}
	}()

	for {
		buf := l.readBuf.swap()
		n, addr, err := l.nconn.ReadFromUDP(buf)
		if err != nil {
			break
		}

		l.p.events <- programEventClientFrameUdp{
			l.trackFlowType,
			addr,
			buf[:n],
		}
	}

	close(l.writeChan)

	close(l.done)
}

func (l *serverUdpListener) close() {
	l.nconn.Close()
	<-l.done
}

func (l *serverUdpListener) write(addr *net.UDPAddr, inbuf []byte) {
	buf := l.writeBuf.swap()
	buf = buf[:len(inbuf)]
	copy(buf, inbuf)

	l.writeChan <- &udpAddrFramePair{
		addr: addr,
		buf:  buf,
	}
}
