package main

import (
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/sdp/v3"
)

const (
	sourceRetryInterval          = 5 * time.Second
	sourceCheckStreamInterval    = 5 * time.Second
	sourceKeepaliveInterval      = 60 * time.Second
	sourceReceiverReportInterval = 10 * time.Second
)

type sourceUdpListenerPair struct {
	rtpl  *sourceUdpListener
	rtcpl *sourceUdpListener
}

type source struct {
	p               *program
	path            string
	u               *url.URL
	proto           streamProtocol
	ready           bool
	clientTracks    []*gortsplib.Track
	serverSdpText   []byte
	serverSdpParsed *sdp.SessionDescription
	rtcpReceivers   []*gortsplib.RtcpReceiver
	readBuf         *doubleBuffer

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

	proto, err := func() (streamProtocol, error) {
		switch sourceProtocol {
		case "udp":
			return streamProtocolUdp, nil

		case "tcp":
			return streamProtocolTcp, nil
		}
		return streamProtocol(0), fmt.Errorf("unsupported protocol '%s'", sourceProtocol)
	}()
	if err != nil {
		return nil, err
	}

	s := &source{
		p:         p,
		path:      path,
		u:         u,
		proto:     proto,
		readBuf:   newDoubleBuffer(512 * 1024),
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

	var nconn net.Conn
	var err error
	dialDone := make(chan struct{})
	go func() {
		nconn, err = net.DialTimeout("tcp", s.u.Host, s.p.conf.ReadTimeout)
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
	defer nconn.Close()

	conn := gortsplib.NewConnClient(gortsplib.ConnClientConf{
		Conn:         nconn,
		ReadTimeout:  s.p.conf.ReadTimeout,
		WriteTimeout: s.p.conf.WriteTimeout,
	})

	_, err = conn.Options(s.u)
	if err != nil {
		s.log("ERR: %s", err)
		return true
	}

	clientTracks, _, err := conn.Describe(s.u)
	if err != nil {
		s.log("ERR: %s", err)
		return true
	}

	// create a filtered SDP that is used by the server (not by the client)
	serverSdpParsed, serverSdpText := sdpForServer(clientTracks)

	s.clientTracks = clientTracks
	s.serverSdpText = serverSdpText
	s.serverSdpParsed = serverSdpParsed

	if s.proto == streamProtocolUdp {
		return s.runUdp(conn)
	} else {
		return s.runTcp(conn)
	}
}

func (s *source) runUdp(conn *gortsplib.ConnClient) bool {
	publisherIp := conn.NetConn().RemoteAddr().(*net.TCPAddr).IP

	var sourceUdpListenerPairs []sourceUdpListenerPair

	defer func() {
		for _, pair := range sourceUdpListenerPairs {
			pair.rtpl.close()
			pair.rtcpl.close()
		}
	}()

	for i, track := range s.clientTracks {
		var rtpPort int
		var rtcpPort int
		var rtpl *sourceUdpListener
		var rtcpl *sourceUdpListener
		func() {
			for {
				// choose two consecutive ports in range 65536-10000
				// rtp must be pair and rtcp odd
				rtpPort = (rand.Intn((65535-10000)/2) * 2) + 10000
				rtcpPort = rtpPort + 1

				var err error
				rtpl, err = newSourceUdpListener(s.p, rtpPort, s, i,
					gortsplib.StreamTypeRtp, publisherIp)
				if err != nil {
					continue
				}

				rtcpl, err = newSourceUdpListener(s.p, rtcpPort, s, i,
					gortsplib.StreamTypeRtcp, publisherIp)
				if err != nil {
					rtpl.close()
					continue
				}

				return
			}
		}()

		rtpServerPort, rtcpServerPort, _, err := conn.SetupUdp(s.u, track, rtpPort, rtcpPort)
		if err != nil {
			s.log("ERR: %s", err)
			rtpl.close()
			rtcpl.close()
			return true
		}

		rtpl.publisherPort = rtpServerPort
		rtcpl.publisherPort = rtcpServerPort

		sourceUdpListenerPairs = append(sourceUdpListenerPairs, sourceUdpListenerPair{
			rtpl:  rtpl,
			rtcpl: rtcpl,
		})
	}

	_, err := conn.Play(s.u)
	if err != nil {
		s.log("ERR: %s", err)
		return true
	}

	s.rtcpReceivers = make([]*gortsplib.RtcpReceiver, len(s.clientTracks))
	for trackId := range s.clientTracks {
		s.rtcpReceivers[trackId] = gortsplib.NewRtcpReceiver()
	}

	for _, pair := range sourceUdpListenerPairs {
		pair.rtpl.start()
		pair.rtcpl.start()
	}

	sendKeepaliveTicker := time.NewTicker(sourceKeepaliveInterval)
	checkStreamTicker := time.NewTicker(sourceCheckStreamInterval)
	receiverReportTicker := time.NewTicker(sourceReceiverReportInterval)

	s.p.events <- programEventStreamerReady{s}

	var ret bool

outer:
	for {
		select {
		case <-s.terminate:
			ret = false
			break outer

		case <-sendKeepaliveTicker.C:
			_, err := conn.Options(s.u)
			if err != nil {
				s.log("ERR: %s", err)
				ret = true
				break outer
			}

		case <-checkStreamTicker.C:
			for trackId := range s.clientTracks {
				if time.Since(s.rtcpReceivers[trackId].LastFrameTime()) >= s.p.conf.StreamDeadAfter {
					s.log("ERR: stream is dead")
					ret = true
					break outer
				}
			}

		case <-receiverReportTicker.C:
			for trackId := range s.clientTracks {
				frame := s.rtcpReceivers[trackId].Report()
				sourceUdpListenerPairs[trackId].rtcpl.writeChan <- &udpAddrBufPair{
					addr: &net.UDPAddr{
						IP:   conn.NetConn().RemoteAddr().(*net.TCPAddr).IP,
						Zone: conn.NetConn().RemoteAddr().(*net.TCPAddr).Zone,
						Port: sourceUdpListenerPairs[trackId].rtcpl.publisherPort,
					},
					buf: frame,
				}
			}
		}
	}

	sendKeepaliveTicker.Stop()
	checkStreamTicker.Stop()
	receiverReportTicker.Stop()

	s.p.events <- programEventStreamerNotReady{s}

	for _, pair := range sourceUdpListenerPairs {
		pair.rtpl.stop()
		pair.rtcpl.stop()
	}

	for trackId := range s.clientTracks {
		s.rtcpReceivers[trackId].Close()
	}

	return ret
}

func (s *source) runTcp(conn *gortsplib.ConnClient) bool {
	for _, track := range s.clientTracks {
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

	s.rtcpReceivers = make([]*gortsplib.RtcpReceiver, len(s.clientTracks))
	for trackId := range s.clientTracks {
		s.rtcpReceivers[trackId] = gortsplib.NewRtcpReceiver()
	}

	s.p.events <- programEventStreamerReady{s}

	frame := &gortsplib.InterleavedFrame{}

	chanConnError := make(chan struct{})
	go func() {
		for {
			frame.Content = s.readBuf.swap()
			frame.Content = frame.Content[:cap(frame.Content)]

			err := conn.ReadFrame(frame)
			if err != nil {
				s.log("ERR: %s", err)
				close(chanConnError)
				break
			}

			s.rtcpReceivers[frame.TrackId].OnFrame(frame.StreamType, frame.Content)
			s.p.events <- programEventStreamerFrame{s, frame.TrackId, frame.StreamType, frame.Content}
		}
	}()

	// a ticker to check the stream is not needed since there's already a deadline
	// on the RTSP reads
	receiverReportTicker := time.NewTicker(sourceReceiverReportInterval)

	var ret bool

outer:
	for {
		select {
		case <-s.terminate:
			ret = false
			break outer

		case <-chanConnError:
			ret = true
			break outer

		case <-receiverReportTicker.C:
			for trackId := range s.clientTracks {
				frame := s.rtcpReceivers[trackId].Report()

				conn.WriteFrame(&gortsplib.InterleavedFrame{
					TrackId:    trackId,
					StreamType: gortsplib.StreamTypeRtcp,
					Content:    frame,
				})
			}
		}
	}

	receiverReportTicker.Stop()

	s.p.events <- programEventStreamerNotReady{s}

	for trackId := range s.clientTracks {
		s.rtcpReceivers[trackId].Close()
	}

	return ret
}

func (s *source) close() {
	close(s.terminate)
	<-s.done
}
