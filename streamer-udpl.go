package main

import (
	"net"
	"time"

	"github.com/aler9/gortsplib"
)

type streamerUdpListener struct {
	p             *program
	streamer      *streamer
	trackId       int
	streamType    gortsplib.StreamType
	publisherIp   net.IP
	publisherPort int
	nconn         *net.UDPConn
	running       bool
	readBuf       *doubleBuffer

	writeChan chan *udpAddrBufPair
	done      chan struct{}
}

func newStreamerUdpListener(p *program, port int, streamer *streamer,
	trackId int, streamType gortsplib.StreamType, publisherIp net.IP) (*streamerUdpListener, error) {
	nconn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: port,
	})
	if err != nil {
		return nil, err
	}

	l := &streamerUdpListener{
		p:           p,
		streamer:    streamer,
		trackId:     trackId,
		streamType:  streamType,
		publisherIp: publisherIp,
		nconn:       nconn,
		readBuf:     newDoubleBuffer(2048),
		writeChan:   make(chan *udpAddrBufPair),
		done:        make(chan struct{}),
	}

	return l, nil
}

func (l *streamerUdpListener) close() {
	l.nconn.Close() // close twice
}

func (l *streamerUdpListener) start() {
	go l.run()
}

func (l *streamerUdpListener) stop() {
	l.nconn.Close()
	<-l.done
}

func (l *streamerUdpListener) run() {
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

		if !l.publisherIp.Equal(addr.IP) || addr.Port != l.publisherPort {
			continue
		}

		l.streamer.RtcpReceivers[l.trackId].OnFrame(l.streamType, buf[:n])
		l.p.events <- programEventStreamerFrame{l.streamer, l.trackId, l.streamType, buf[:n]}
	}

	close(l.writeChan)
	<-writeDone

	close(l.done)
}
