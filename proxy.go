package main

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
)

const (
	proxyRetryInterval = 5 * time.Second
)

type proxyState int

const (
	proxyStateStopped proxyState = iota
	proxyStateRunning
)

type proxy struct {
	p            *program
	path         *path
	pathConf     *pathConf
	state        proxyState
	tracks       []*gortsplib.Track
	innerRunning bool

	innerTerminate chan struct{}
	innerDone      chan struct{}
	setState       chan proxyState
	terminate      chan struct{}
	done           chan struct{}
}

func newProxy(p *program, path *path, pathConf *pathConf) *proxy {
	s := &proxy{
		p:         p,
		path:      path,
		pathConf:  pathConf,
		setState:  make(chan proxyState),
		terminate: make(chan struct{}),
		done:      make(chan struct{}),
	}

	atomic.AddInt64(p.countProxies, +1)

	if pathConf.SourceOnDemand {
		s.state = proxyStateStopped
	} else {
		s.state = proxyStateRunning
		atomic.AddInt64(p.countProxiesRunning, +1)
	}

	return s
}

func (s *proxy) isPublisher() {}

func (s *proxy) run(initialState proxyState) {
	s.applyState(initialState)

outer:
	for {
		select {
		case state := <-s.setState:
			s.applyState(state)

		case <-s.terminate:
			break outer
		}
	}

	if s.innerRunning {
		close(s.innerTerminate)
		<-s.innerDone
	}

	close(s.setState)
	close(s.done)
}

func (s *proxy) applyState(state proxyState) {
	if state == proxyStateRunning {
		if !s.innerRunning {
			s.path.log("proxy started")
			s.innerRunning = true
			s.innerTerminate = make(chan struct{})
			s.innerDone = make(chan struct{})
			go s.runInner()
		}
	} else {
		if s.innerRunning {
			close(s.innerTerminate)
			<-s.innerDone
			s.innerRunning = false
			s.path.log("proxy stopped")
		}
	}
}

func (s *proxy) runInner() {
	defer close(s.innerDone)

	for {
		ok := func() bool {
			ok := s.runInnerInner()
			if !ok {
				return false
			}

			t := time.NewTimer(proxyRetryInterval)
			defer t.Stop()

			select {
			case <-s.innerTerminate:
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

func (s *proxy) runInnerInner() bool {
	s.path.log("proxy connecting")

	var conn *gortsplib.ConnClient
	var err error
	dialDone := make(chan struct{})
	go func() {
		conn, err = gortsplib.NewConnClient(gortsplib.ConnClientConf{
			Host:            s.pathConf.sourceUrl.Host,
			ReadTimeout:     s.p.conf.ReadTimeout,
			WriteTimeout:    s.p.conf.WriteTimeout,
			ReadBufferCount: 2,
		})
		close(dialDone)
	}()

	select {
	case <-s.innerTerminate:
		return false
	case <-dialDone:
	}

	if err != nil {
		s.path.log("proxy ERR: %s", err)
		return true
	}

	_, err = conn.Options(s.pathConf.sourceUrl)
	if err != nil {
		conn.Close()
		s.path.log("proxy ERR: %s", err)
		return true
	}

	tracks, _, err := conn.Describe(s.pathConf.sourceUrl)
	if err != nil {
		conn.Close()
		s.path.log("proxy ERR: %s", err)
		return true
	}

	// create a filtered SDP that is used by the server (not by the client)
	serverSdp := tracks.Write()

	s.tracks = tracks
	s.path.publisherTrackCount = len(tracks)
	s.path.publisherSdp = serverSdp

	if s.pathConf.sourceProtocolParsed == gortsplib.StreamProtocolUDP {
		return s.runUDP(conn)
	} else {
		return s.runTCP(conn)
	}
}

func (s *proxy) runUDP(conn *gortsplib.ConnClient) bool {
	var rtpReads []gortsplib.UDPReadFunc
	var rtcpReads []gortsplib.UDPReadFunc

	for _, track := range s.tracks {
		rtpRead, rtcpRead, _, err := conn.SetupUDP(s.pathConf.sourceUrl, track, 0, 0)
		if err != nil {
			conn.Close()
			s.path.log("proxy ERR: %s", err)
			return true
		}

		rtpReads = append(rtpReads, rtpRead)
		rtcpReads = append(rtcpReads, rtcpRead)
	}

	_, err := conn.Play(s.pathConf.sourceUrl)
	if err != nil {
		conn.Close()
		s.path.log("proxy ERR: %s", err)
		return true
	}

	s.p.proxyReady <- s

	var wg sync.WaitGroup

	// receive RTP packets
	for trackId, rtpRead := range rtpReads {
		wg.Add(1)
		go func(trackId int, rtpRead gortsplib.UDPReadFunc) {
			defer wg.Done()

			for {
				buf, err := rtpRead()
				if err != nil {
					break
				}

				s.p.readersMap.forwardFrame(s.path, trackId,
					gortsplib.StreamTypeRtp, buf)
			}
		}(trackId, rtpRead)
	}

	// receive RTCP packets
	for trackId, rtcpRead := range rtcpReads {
		wg.Add(1)
		go func(trackId int, rtcpRead gortsplib.UDPReadFunc) {
			defer wg.Done()

			for {
				buf, err := rtcpRead()
				if err != nil {
					break
				}

				s.p.readersMap.forwardFrame(s.path, trackId,
					gortsplib.StreamTypeRtcp, buf)
			}
		}(trackId, rtcpRead)
	}

	tcpConnDone := make(chan error)
	go func() {
		tcpConnDone <- conn.LoopUDP(s.pathConf.sourceUrl)
	}()

	var ret bool

outer:
	for {
		select {
		case <-s.innerTerminate:
			conn.Close()
			<-tcpConnDone
			ret = false
			break outer

		case err := <-tcpConnDone:
			conn.Close()
			s.path.log("proxy ERR: %s", err)
			ret = true
			break outer
		}
	}

	wg.Wait()

	s.p.proxyNotReady <- s

	return ret
}

func (s *proxy) runTCP(conn *gortsplib.ConnClient) bool {
	for _, track := range s.tracks {
		_, err := conn.SetupTCP(s.pathConf.sourceUrl, track)
		if err != nil {
			conn.Close()
			s.path.log("proxy ERR: %s", err)
			return true
		}
	}

	_, err := conn.Play(s.pathConf.sourceUrl)
	if err != nil {
		conn.Close()
		s.path.log("proxy ERR: %s", err)
		return true
	}

	s.p.proxyReady <- s

	tcpConnDone := make(chan error)
	go func() {
		for {
			frame, err := conn.ReadFrame()
			if err != nil {
				tcpConnDone <- err
				return
			}

			s.p.readersMap.forwardFrame(s.path, frame.TrackId, frame.StreamType, frame.Content)
		}
	}()

	var ret bool

outer:
	for {
		select {
		case <-s.innerTerminate:
			conn.Close()
			<-tcpConnDone
			ret = false
			break outer

		case err := <-tcpConnDone:
			conn.Close()
			s.path.log("proxy ERR: %s", err)
			ret = true
			break outer
		}
	}

	s.p.proxyNotReady <- s

	return ret
}
