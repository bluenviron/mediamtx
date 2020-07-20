package main

import (
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/sdp/v3"
)

const (
	sourceRetryInterval     = 5 * time.Second
	sourceUdpReadBufferSize = 2048
	sourceTcpReadBufferSize = 128 * 1024
)

type source struct {
	p               *program
	path            string
	u               *url.URL
	proto           gortsplib.StreamProtocol
	ready           bool
	tracks          []*gortsplib.Track
	serverSdpText   []byte
	serverSdpParsed *sdp.SessionDescription

	terminate chan struct{}
	done      chan struct{}
}

func newSource(p *program, path string, sourceStr string, sourceProtocol string) (*source, error) {
	u, err := url.Parse(sourceStr)
	if err != nil {
		return nil, fmt.Errorf("'%s' is not a valid RTSP url", sourceStr)
	}
	if u.Scheme != "rtsp" {
		return nil, fmt.Errorf("'%s' is not a valid RTSP url", sourceStr)
	}
	if u.Port() == "" {
		u.Host += ":554"
	}
	if u.User != nil {
		pass, _ := u.User.Password()
		user := u.User.Username()
		if user != "" && pass == "" ||
			user == "" && pass != "" {
			fmt.Errorf("username and password must be both provided")
		}
	}

	proto, err := func() (gortsplib.StreamProtocol, error) {
		switch sourceProtocol {
		case "udp":
			return gortsplib.StreamProtocolUdp, nil

		case "tcp":
			return gortsplib.StreamProtocolTcp, nil
		}
		return gortsplib.StreamProtocol(0), fmt.Errorf("unsupported protocol '%s'", sourceProtocol)
	}()
	if err != nil {
		return nil, err
	}

	s := &source{
		p:         p,
		path:      path,
		u:         u,
		proto:     proto,
		terminate: make(chan struct{}),
		done:      make(chan struct{}),
	}

	return s, nil
}

func (s *source) log(format string, args ...interface{}) {
	s.p.log("[source "+s.path+"] "+format, args...)
}

func (s *source) publisherIsReady() bool {
	return s.ready
}

func (s *source) publisherSdpText() []byte {
	return s.serverSdpText
}

func (s *source) publisherSdpParsed() *sdp.SessionDescription {
	return s.serverSdpParsed
}

func (s *source) run() {
	for {
		ok := s.do()
		if !ok {
			break
		}

		t := time.NewTimer(sourceRetryInterval)
		select {
		case <-s.terminate:
			break
		case <-t.C:
		}
	}

	close(s.done)
}

func (s *source) do() bool {
	s.log("initializing with protocol %s", s.proto)

	var conn *gortsplib.ConnClient
	var err error
	dialDone := make(chan struct{})
	go func() {
		conn, err = gortsplib.NewConnClient(gortsplib.ConnClientConf{
			Host:         s.u.Host,
			ReadTimeout:  s.p.conf.ReadTimeout,
			WriteTimeout: s.p.conf.WriteTimeout,
		})
		close(dialDone)
	}()

	select {
	case <-s.terminate:
		return false
	case <-dialDone:
	}

	if err != nil {
		s.log("ERR: %s", err)
		return true
	}

	defer conn.Close()

	_, err = conn.Options(s.u)
	if err != nil {
		s.log("ERR: %s", err)
		return true
	}

	tracks, _, err := conn.Describe(s.u)
	if err != nil {
		s.log("ERR: %s", err)
		return true
	}

	// create a filtered SDP that is used by the server (not by the client)
	serverSdpParsed, serverSdpText := sdpForServer(tracks)

	s.tracks = tracks
	s.serverSdpText = serverSdpText
	s.serverSdpParsed = serverSdpParsed

	if s.proto == gortsplib.StreamProtocolUdp {
		return s.runUdp(conn)
	} else {
		return s.runTcp(conn)
	}
}

func (s *source) runUdp(conn *gortsplib.ConnClient) bool {
	type trackListenerPair struct {
		rtpl  *gortsplib.ConnClientUdpListener
		rtcpl *gortsplib.ConnClientUdpListener
	}
	var listeners []*trackListenerPair

	for _, track := range s.tracks {
		var rtpl *gortsplib.ConnClientUdpListener
		var rtcpl *gortsplib.ConnClientUdpListener
		var err error

		for {
			// choose two consecutive ports in range 65536-10000
			// rtp must be pair and rtcp odd
			rtpPort := (rand.Intn((65535-10000)/2) * 2) + 10000
			rtcpPort := rtpPort + 1

			rtpl, rtcpl, _, err = conn.SetupUdp(s.u, track, rtpPort, rtcpPort)
			if err != nil {
				// retry if it's a bind error
				if nerr, ok := err.(*net.OpError); ok {
					if serr, ok := nerr.Err.(*os.SyscallError); ok {
						if serr.Syscall == "bind" {
							continue
						}
					}
				}

				s.log("ERR: %s", err)
				return true
			}

			break
		}

		listeners = append(listeners, &trackListenerPair{
			rtpl:  rtpl,
			rtcpl: rtcpl,
		})
	}

	_, err := conn.Play(s.u)
	if err != nil {
		s.log("ERR: %s", err)
		return true
	}

	s.p.events <- programEventStreamerReady{s}

	var wg sync.WaitGroup

	for trackId, lp := range listeners {
		wg.Add(2)

		// receive RTP packets
		go func(trackId int, l *gortsplib.ConnClientUdpListener) {
			defer wg.Done()

			doubleBuf := newDoubleBuffer(sourceUdpReadBufferSize)
			for {
				buf := doubleBuf.swap()

				n, err := l.Read(buf)
				if err != nil {
					break
				}

				s.p.events <- programEventStreamerFrame{s, trackId, gortsplib.StreamTypeRtp, buf[:n]}
			}
		}(trackId, lp.rtpl)

		// receive RTCP packets
		go func(trackId int, l *gortsplib.ConnClientUdpListener) {
			defer wg.Done()

			doubleBuf := newDoubleBuffer(sourceUdpReadBufferSize)
			for {
				buf := doubleBuf.swap()

				n, err := l.Read(buf)
				if err != nil {
					break
				}

				s.p.events <- programEventStreamerFrame{s, trackId, gortsplib.StreamTypeRtcp, buf[:n]}
			}
		}(trackId, lp.rtcpl)
	}

	tcpConnDone := make(chan error)
	go func() {
		tcpConnDone <- conn.LoopUDP(s.u)
	}()

	var ret bool

outer:
	for {
		select {
		case <-s.terminate:
			conn.NetConn().Close()
			<-tcpConnDone
			ret = false
			break outer

		case err := <-tcpConnDone:
			s.log("ERR: %s", err)
			ret = true
			break outer
		}
	}

	s.p.events <- programEventStreamerNotReady{s}

	for _, lp := range listeners {
		lp.rtpl.Close()
		lp.rtcpl.Close()
	}
	wg.Wait()

	return ret
}

func (s *source) runTcp(conn *gortsplib.ConnClient) bool {
	for _, track := range s.tracks {
		_, err := conn.SetupTcp(s.u, track)
		if err != nil {
			s.log("ERR: %s", err)
			return true
		}
	}

	_, err := conn.Play(s.u)
	if err != nil {
		s.log("ERR: %s", err)
		return true
	}

	s.p.events <- programEventStreamerReady{s}

	frame := &gortsplib.InterleavedFrame{}
	doubleBuf := newDoubleBuffer(sourceTcpReadBufferSize)

	tcpConnDone := make(chan error)
	go func() {
		for {
			frame.Content = doubleBuf.swap()
			frame.Content = frame.Content[:cap(frame.Content)]

			err := conn.ReadFrame(frame)
			if err != nil {
				tcpConnDone <- err
				return
			}

			s.p.events <- programEventStreamerFrame{s, frame.TrackId, frame.StreamType, frame.Content}
		}
	}()

	var ret bool

outer:
	for {
		select {
		case <-s.terminate:
			conn.NetConn().Close()
			<-tcpConnDone
			ret = false
			break outer

		case err := <-tcpConnDone:
			s.log("ERR: %s", err)
			ret = true
			break outer
		}
	}

	s.p.events <- programEventStreamerNotReady{s}

	return ret
}

func (s *source) close() {
	close(s.terminate)
	<-s.done
}
