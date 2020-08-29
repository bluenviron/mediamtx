package main

import (
	"math/rand"
	"net"
	"os"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
)

const (
	sourceRetryInterval     = 5 * time.Second
	sourceUdpReadBufferSize = 2048
	sourceTcpReadBufferSize = 128 * 1024
)

type sourceState int

const (
	sourceStateStopped sourceState = iota
	sourceStateRunning
)

type sourceEvent interface {
	isSourceEvent()
}

type sourceEventApplyState struct {
	state sourceState
}

func (sourceEventApplyState) isSourceEvent() {}

type sourceEventTerminate struct{}

func (sourceEventTerminate) isSourceEvent() {}

type source struct {
	p        *program
	pathName string
	confp    *confPath
	state    sourceState
	tracks   []*gortsplib.Track

	events chan sourceEvent
	done   chan struct{}
}

func newSource(p *program, pathName string, confp *confPath) *source {
	s := &source{
		p:        p,
		pathName: pathName,
		confp:    confp,
		events:   make(chan sourceEvent),
		done:     make(chan struct{}),
	}

	if confp.SourceOnDemand {
		s.state = sourceStateStopped
	} else {
		s.state = sourceStateRunning
	}

	return s
}

func (s *source) log(format string, args ...interface{}) {
	s.p.log("[source "+s.pathName+"] "+format, args...)
}

func (s *source) isPublisher() {}

func (s *source) run() {
	running := false
	var doTerminate chan struct{}
	var doDone chan struct{}

	applyState := func(state sourceState) {
		if state == sourceStateRunning {
			if !running {
				s.log("started")
				running = true
				doTerminate = make(chan struct{})
				doDone = make(chan struct{})
				go s.do(doTerminate, doDone)
			}
		} else {
			if running {
				close(doTerminate)
				<-doDone
				running = false
				s.log("stopped")
			}
		}
	}

	applyState(s.state)

outer:
	for rawEvt := range s.events {
		switch evt := rawEvt.(type) {
		case sourceEventApplyState:
			applyState(evt.state)

		case sourceEventTerminate:
			break outer
		}
	}

	if running {
		close(doTerminate)
		<-doDone
	}

	close(s.done)
}

func (s *source) do(terminate chan struct{}, done chan struct{}) {
	defer close(done)

	for {
		ok := func() bool {
			ok := s.doInner(terminate)
			if !ok {
				return false
			}

			t := time.NewTimer(sourceRetryInterval)
			defer t.Stop()

			select {
			case <-terminate:
				return false
			case <-t.C:
			}

			return true
		}()
		if !ok {
			break
		}
	}
}

func (s *source) doInner(terminate chan struct{}) bool {
	s.log("connecting")

	var conn *gortsplib.ConnClient
	var err error
	dialDone := make(chan struct{})
	go func() {
		conn, err = gortsplib.NewConnClient(gortsplib.ConnClientConf{
			Host:         s.confp.sourceUrl.Host,
			ReadTimeout:  s.p.conf.ReadTimeout,
			WriteTimeout: s.p.conf.WriteTimeout,
		})
		close(dialDone)
	}()

	select {
	case <-terminate:
		return false
	case <-dialDone:
	}

	if err != nil {
		s.log("ERR: %s", err)
		return true
	}

	_, err = conn.Options(s.confp.sourceUrl)
	if err != nil {
		conn.Close()
		s.log("ERR: %s", err)
		return true
	}

	tracks, _, err := conn.Describe(s.confp.sourceUrl)
	if err != nil {
		conn.Close()
		s.log("ERR: %s", err)
		return true
	}

	// create a filtered SDP that is used by the server (not by the client)
	serverSdpParsed, serverSdpText := sdpForServer(tracks)

	s.tracks = tracks
	s.p.paths[s.pathName].publisherSdpText = serverSdpText
	s.p.paths[s.pathName].publisherSdpParsed = serverSdpParsed

	if s.confp.sourceProtocolParsed == gortsplib.StreamProtocolUdp {
		return s.runUdp(terminate, conn)
	} else {
		return s.runTcp(terminate, conn)
	}
}

func (s *source) runUdp(terminate chan struct{}, conn *gortsplib.ConnClient) bool {
	var rtpReads []gortsplib.UdpReadFunc
	var rtcpReads []gortsplib.UdpReadFunc

	for _, track := range s.tracks {
		for {
			// choose two consecutive ports in range 65536-10000
			// rtp must be even and rtcp odd
			rtpPort := (rand.Intn((65535-10000)/2) * 2) + 10000
			rtcpPort := rtpPort + 1

			rtpRead, rtcpRead, _, err := conn.SetupUdp(s.confp.sourceUrl, track, rtpPort, rtcpPort)
			if err != nil {
				// retry if it's a bind error
				if nerr, ok := err.(*net.OpError); ok {
					if serr, ok := nerr.Err.(*os.SyscallError); ok {
						if serr.Syscall == "bind" {
							continue
						}
					}
				}

				conn.Close()
				s.log("ERR: %s", err)
				return true
			}

			rtpReads = append(rtpReads, rtpRead)
			rtcpReads = append(rtcpReads, rtcpRead)
			break
		}
	}

	_, err := conn.Play(s.confp.sourceUrl)
	if err != nil {
		conn.Close()
		s.log("ERR: %s", err)
		return true
	}

	s.p.events <- programEventSourceReady{s}

	var wg sync.WaitGroup

	// receive RTP packets
	for trackId, rtpRead := range rtpReads {
		wg.Add(1)
		go func(trackId int, rtpRead gortsplib.UdpReadFunc) {
			defer wg.Done()

			multiBuf := newMultiBuffer(3, sourceUdpReadBufferSize)
			for {
				buf := multiBuf.next()

				n, err := rtpRead(buf)
				if err != nil {
					break
				}

				s.p.events <- programEventSourceFrame{s, trackId, gortsplib.StreamTypeRtp, buf[:n]}
			}
		}(trackId, rtpRead)
	}

	// receive RTCP packets
	for trackId, rtcpRead := range rtcpReads {
		wg.Add(1)
		go func(trackId int, rtcpRead gortsplib.UdpReadFunc) {
			defer wg.Done()

			multiBuf := newMultiBuffer(3, sourceUdpReadBufferSize)
			for {
				buf := multiBuf.next()

				n, err := rtcpRead(buf)
				if err != nil {
					break
				}

				s.p.events <- programEventSourceFrame{s, trackId, gortsplib.StreamTypeRtcp, buf[:n]}
			}
		}(trackId, rtcpRead)
	}

	tcpConnDone := make(chan error)
	go func() {
		tcpConnDone <- conn.LoopUdp(s.confp.sourceUrl)
	}()

	var ret bool

outer:
	for {
		select {
		case <-terminate:
			conn.Close()
			<-tcpConnDone
			ret = false
			break outer

		case err := <-tcpConnDone:
			conn.Close()
			s.log("ERR: %s", err)
			ret = true
			break outer
		}
	}

	wg.Wait()

	s.p.events <- programEventSourceNotReady{s}

	return ret
}

func (s *source) runTcp(terminate chan struct{}, conn *gortsplib.ConnClient) bool {
	for _, track := range s.tracks {
		_, err := conn.SetupTcp(s.confp.sourceUrl, track)
		if err != nil {
			conn.Close()
			s.log("ERR: %s", err)
			return true
		}
	}

	_, err := conn.Play(s.confp.sourceUrl)
	if err != nil {
		conn.Close()
		s.log("ERR: %s", err)
		return true
	}

	s.p.events <- programEventSourceReady{s}

	frame := &gortsplib.InterleavedFrame{}
	multiBuf := newMultiBuffer(3, sourceTcpReadBufferSize)

	tcpConnDone := make(chan error)
	go func() {
		for {
			frame.Content = multiBuf.next()
			frame.Content = frame.Content[:cap(frame.Content)]

			err := conn.ReadFrame(frame)
			if err != nil {
				tcpConnDone <- err
				return
			}

			s.p.events <- programEventSourceFrame{s, frame.TrackId, frame.StreamType, frame.Content}
		}
	}()

	var ret bool

outer:
	for {
		select {
		case <-terminate:
			conn.Close()
			<-tcpConnDone
			ret = false
			break outer

		case err := <-tcpConnDone:
			conn.Close()
			s.log("ERR: %s", err)
			ret = true
			break outer
		}
	}

	s.p.events <- programEventSourceNotReady{s}

	return ret
}
