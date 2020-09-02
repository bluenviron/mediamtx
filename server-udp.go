package main

import (
	"net"
	"time"

	"github.com/aler9/gortsplib"
)

type udpBufAddrPair struct {
	buf  []byte
	addr *net.UDPAddr
}

type serverUdp struct {
	p          *program
	pc         *net.UDPConn
	streamType gortsplib.StreamType
	readBuf    *multiBuffer

	writec chan udpBufAddrPair
	done   chan struct{}
}

func newServerUdp(p *program, port int, streamType gortsplib.StreamType) (*serverUdp, error) {
	pc, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: port,
	})
	if err != nil {
		return nil, err
	}

	l := &serverUdp{
		p:          p,
		pc:         pc,
		streamType: streamType,
		readBuf:    newMultiBuffer(3, clientUdpReadBufferSize),
		writec:     make(chan udpBufAddrPair),
		done:       make(chan struct{}),
	}

	l.log("opened on :%d", port)
	return l, nil
}

func (l *serverUdp) log(format string, args ...interface{}) {
	var label string
	if l.streamType == gortsplib.StreamTypeRtp {
		label = "RTP"
	} else {
		label = "RTCP"
	}
	l.p.log("[UDP/"+label+" listener] "+format, args...)
}

func (l *serverUdp) run() {
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		for w := range l.writec {
			l.pc.SetWriteDeadline(time.Now().Add(l.p.conf.WriteTimeout))
			l.pc.WriteTo(w.buf, w.addr)
		}
	}()

	for {
		buf := l.readBuf.next()
		n, addr, err := l.pc.ReadFromUDP(buf)
		if err != nil {
			break
		}

		l.p.clientFrameUdp <- clientFrameUdpReq{
			addr,
			l.streamType,
			buf[:n],
		}
	}

	close(l.writec)
	<-writeDone

	close(l.done)
}

func (l *serverUdp) close() {
	l.pc.Close()
	<-l.done
}

func (l *serverUdp) write(data []byte, addr *net.UDPAddr) {
	l.writec <- udpBufAddrPair{data, addr}
}
