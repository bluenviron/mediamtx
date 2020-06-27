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
	readBuf1      []byte
	readBuf2      []byte
	readCurBuf    bool
	writeBuf1     []byte
	writeBuf2     []byte
	writeCurBuf   bool

	writec chan *udpWrite
	done   chan struct{}
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
		readBuf1:      make([]byte, 2048),
		readBuf2:      make([]byte, 2048),
		writeBuf1:     make([]byte, 2048),
		writeBuf2:     make([]byte, 2048),
		writec:        make(chan *udpWrite),
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
		for w := range l.writec {
			l.nconn.SetWriteDeadline(time.Now().Add(l.p.args.writeTimeout))
			l.nconn.WriteTo(w.buf, w.addr)
		}
	}()

	for {
		var buf []byte
		if !l.readCurBuf {
			buf = l.readBuf1
		} else {
			buf = l.readBuf2
		}
		l.readCurBuf = !l.readCurBuf

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

	close(l.writec)

	close(l.done)
}

func (l *serverUdpListener) close() {
	l.nconn.Close()
	<-l.done
}

func (l *serverUdpListener) write(addr *net.UDPAddr, inbuf []byte) {
	var buf []byte
	if !l.writeCurBuf {
		buf = l.writeBuf1
	} else {
		buf = l.writeBuf2
	}

	buf = buf[:len(inbuf)]
	copy(buf, inbuf)
	l.writeCurBuf = !l.writeCurBuf

	l.writec <- &udpWrite{
		addr: addr,
		buf:  buf,
	}
}
