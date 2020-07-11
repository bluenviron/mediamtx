package main

import (
	"net"
	"time"
)

type streamerUdpListener struct {
	p             *program
	streamer      *streamer
	trackId       int
	trackFlowType trackFlowType
	publisherIp   net.IP
	publisherPort int
	nconn         *net.UDPConn
	running       bool
	readBuf       *doubleBuffer
	lastFrameTime time.Time

	done chan struct{}
}

func newStreamerUdpListener(p *program, port int, streamer *streamer,
	trackId int, trackFlowType trackFlowType, publisherIp net.IP) (*streamerUdpListener, error) {
	nconn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: port,
	})
	if err != nil {
		return nil, err
	}

	l := &streamerUdpListener{
		p:             p,
		streamer:      streamer,
		trackId:       trackId,
		trackFlowType: trackFlowType,
		publisherIp:   publisherIp,
		nconn:         nconn,
		readBuf:       newDoubleBuffer(2048),
		lastFrameTime: time.Now(),
		done:          make(chan struct{}),
	}

	return l, nil
}

func (l *streamerUdpListener) close() {
	l.nconn.Close()

	if l.running {
		<-l.done
	}
}

func (l *streamerUdpListener) start() {
	l.running = true
	go l.run()
}

func (l *streamerUdpListener) run() {
	for {
		buf := l.readBuf.swap()
		n, addr, err := l.nconn.ReadFromUDP(buf)
		if err != nil {
			break
		}

		if !l.publisherIp.Equal(addr.IP) || addr.Port != l.publisherPort {
			continue
		}

		l.lastFrameTime = time.Now()

		l.p.events <- programEventStreamerFrame{l.streamer, l.trackId, l.trackFlowType, buf[:n]}
	}

	close(l.done)
}
