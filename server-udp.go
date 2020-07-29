package main

import (
	"net"
	"time"

	"github.com/aler9/gortsplib"
)

type udpAddrBufPair struct {
	addr *net.UDPAddr
	buf  []byte
}

type serverUdp struct {
	p          *program
	nconn      *net.UDPConn
	streamType gortsplib.StreamType
	readBuf    *doubleBuffer
	writeBuf   *doubleBuffer

	writeChan chan *udpAddrBufPair
	done      chan struct{}
}

func newServerUdp(p *program, port int, streamType gortsplib.StreamType) (*serverUdp, error) {
	nconn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: port,
	})
	if err != nil {
		return nil, err
	}

	l := &serverUdp{
		p:          p,
		nconn:      nconn,
		streamType: streamType,
		readBuf:    newDoubleBuffer(clientUdpReadBufferSize),
		writeBuf:   newDoubleBuffer(clientUdpWriteBufferSize),
		writeChan:  make(chan *udpAddrBufPair),
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
			addr,
			l.streamType,
			buf[:n],
		}
	}

	close(l.writeChan)
	<-writeDone

	close(l.done)
}

func (l *serverUdp) close() {
	l.nconn.Close()
	<-l.done
}

func (l *serverUdp) write(pair *udpAddrBufPair) {
	// replace input buffer with write buffer
	buf := l.writeBuf.swap()
	buf = buf[:len(pair.buf)]
	copy(buf, pair.buf)
	pair.buf = buf

	l.writeChan <- pair
}
