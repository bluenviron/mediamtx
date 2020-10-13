package main

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
)

const (
	sourceRtspRetryInterval = 5 * time.Second
)

type sourceRtspState int

const (
	sourceRtspStateStopped sourceRtspState = iota
	sourceRtspStateRunning
)

type sourceRtsp struct {
	p            *program
	path         *path
	state        sourceRtspState
	tracks       []*gortsplib.Track
	innerRunning bool

	innerTerminate chan struct{}
	innerDone      chan struct{}
	setState       chan sourceRtspState
	terminate      chan struct{}
	done           chan struct{}
}

func newSourceRtsp(p *program, path *path) *sourceRtsp {
	s := &sourceRtsp{
		p:         p,
		path:      path,
		setState:  make(chan sourceRtspState),
		terminate: make(chan struct{}),
		done:      make(chan struct{}),
	}

	atomic.AddInt64(p.countSourcesRtsp, +1)

	if path.conf.SourceOnDemand {
		s.state = sourceRtspStateStopped
	} else {
		s.state = sourceRtspStateRunning
		atomic.AddInt64(p.countSourcesRtspRunning, +1)
	}

	return s
}

func (s *sourceRtsp) isSource() {}

func (s *sourceRtsp) run(initialState sourceRtspState) {
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

func (s *sourceRtsp) applyState(state sourceRtspState) {
	if state == sourceRtspStateRunning {
		if !s.innerRunning {
			s.path.log("rtsp source started")
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
			s.path.log("rtsp source stopped")
		}
	}
}

func (s *sourceRtsp) runInner() {
	defer close(s.innerDone)

outer:
	for {
		ok := s.runInnerInner()
		if !ok {
			break outer
		}

		t := time.NewTimer(sourceRtspRetryInterval)
		defer t.Stop()

		select {
		case <-s.innerTerminate:
			break outer
		case <-t.C:
		}
	}
}

func (s *sourceRtsp) runInnerInner() bool {
	s.path.log("connecting to rtsp source")

	var conn *gortsplib.ConnClient
	var err error
	dialDone := make(chan struct{}, 1)
	go func() {
		defer close(dialDone)
		conn, err = gortsplib.NewConnClient(gortsplib.ConnClientConf{
			Host:            s.path.conf.SourceUrl.Host,
			ReadTimeout:     s.p.conf.ReadTimeout,
			WriteTimeout:    s.p.conf.WriteTimeout,
			ReadBufferCount: 2,
		})
	}()

	select {
	case <-s.innerTerminate:
		return false
	case <-dialDone:
	}

	if err != nil {
		s.path.log("rtsp source ERR: %s", err)
		return true
	}

	_, err = conn.Options(s.path.conf.SourceUrl)
	if err != nil {
		conn.Close()
		s.path.log("rtsp source ERR: %s", err)
		return true
	}

	tracks, _, err := conn.Describe(s.path.conf.SourceUrl)
	if err != nil {
		conn.Close()
		s.path.log("rtsp source ERR: %s", err)
		return true
	}

	// create a filtered SDP that is used by the server (not by the client)
	s.path.sourceSdp = tracks.Write()
	s.path.sourceTrackCount = len(tracks)
	s.tracks = tracks

	if s.path.conf.SourceProtocolParsed == gortsplib.StreamProtocolUDP {
		return s.runUDP(conn)
	} else {
		return s.runTCP(conn)
	}
}

func (s *sourceRtsp) runUDP(conn *gortsplib.ConnClient) bool {
	for _, track := range s.tracks {
		_, err := conn.SetupUDP(s.path.conf.SourceUrl, gortsplib.TransportModePlay, track, 0, 0)
		if err != nil {
			conn.Close()
			s.path.log("rtsp source ERR: %s", err)
			return true
		}
	}

	_, err := conn.Play(s.path.conf.SourceUrl)
	if err != nil {
		conn.Close()
		s.path.log("rtsp source ERR: %s", err)
		return true
	}

	s.p.sourceRtspReady <- s
	s.path.log("rtsp source ready")

	var wg sync.WaitGroup

	// receive RTP packets
	for trackId := range s.tracks {
		wg.Add(1)
		go func(trackId int) {
			defer wg.Done()

			for {
				buf, err := conn.ReadFrameUDP(trackId, gortsplib.StreamTypeRtp)
				if err != nil {
					break
				}

				s.p.readersMap.forwardFrame(s.path, trackId,
					gortsplib.StreamTypeRtp, buf)
			}
		}(trackId)
	}

	// receive RTCP packets
	for trackId := range s.tracks {
		wg.Add(1)
		go func(trackId int) {
			defer wg.Done()

			for {
				buf, err := conn.ReadFrameUDP(trackId, gortsplib.StreamTypeRtcp)
				if err != nil {
					break
				}

				s.p.readersMap.forwardFrame(s.path, trackId,
					gortsplib.StreamTypeRtcp, buf)
			}
		}(trackId)
	}

	tcpConnDone := make(chan error)
	go func() {
		tcpConnDone <- conn.LoopUDP()
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
			s.path.log("rtsp source ERR: %s", err)
			ret = true
			break outer
		}
	}

	wg.Wait()

	s.p.sourceRtspNotReady <- s
	s.path.log("rtsp source not ready")

	return ret
}

func (s *sourceRtsp) runTCP(conn *gortsplib.ConnClient) bool {
	for _, track := range s.tracks {
		_, err := conn.SetupTCP(s.path.conf.SourceUrl, gortsplib.TransportModePlay, track)
		if err != nil {
			conn.Close()
			s.path.log("rtsp source ERR: %s", err)
			return true
		}
	}

	_, err := conn.Play(s.path.conf.SourceUrl)
	if err != nil {
		conn.Close()
		s.path.log("rtsp source ERR: %s", err)
		return true
	}

	s.p.sourceRtspReady <- s
	s.path.log("rtsp source ready")

	tcpConnDone := make(chan error)
	go func() {
		for {
			trackId, streamType, content, err := conn.ReadFrameTCP()
			if err != nil {
				tcpConnDone <- err
				return
			}

			s.p.readersMap.forwardFrame(s.path, trackId, streamType, content)
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
			s.path.log("rtsp source ERR: %s", err)
			ret = true
			break outer
		}
	}

	s.p.sourceRtspNotReady <- s
	s.path.log("rtsp source not ready")

	return ret
}
