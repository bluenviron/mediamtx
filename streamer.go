package main

import (
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/pion/sdp"
)

const (
	_STREAMER_RETRY_INTERVAL           = 5 * time.Second
	_STREAMER_CHECK_STREAM_INTERVAL    = 5 * time.Second
	_STREAMER_KEEPALIVE_INTERVAL       = 60 * time.Second
	_STREAMER_RECEIVER_REPORT_INTERVAL = 10 * time.Second
)

type streamerUdpListenerPair struct {
	rtpl  *streamerUdpListener
	rtcpl *streamerUdpListener
}

type streamer struct {
	p               *program
	path            string
	u               *url.URL
	proto           streamProtocol
	ready           bool
	clientSdpParsed *sdp.SessionDescription
	serverSdpText   []byte
	serverSdpParsed *sdp.SessionDescription
	rtcpReceivers   []*rtcpReceiver
	readBuf         *doubleBuffer

	terminate chan struct{}
	done      chan struct{}
}

func newStreamer(p *program, path string, source string, sourceProtocol string) (*streamer, error) {
	u, err := url.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("'%s' is not a valid source not an RTSP url", source)
	}
	if u.Scheme != "rtsp" {
		return nil, fmt.Errorf("'%s' is not a valid RTSP url", source)
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
			return _STREAM_PROTOCOL_UDP, nil

		case "tcp":
			return _STREAM_PROTOCOL_TCP, nil
		}
		return streamProtocol(0), fmt.Errorf("unsupported protocol '%s'", sourceProtocol)
	}()
	if err != nil {
		return nil, err
	}

	s := &streamer{
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

func (s *streamer) log(format string, args ...interface{}) {
	s.p.log("[streamer "+s.path+"] "+format, args...)
}

func (s *streamer) publisherIsReady() bool {
	return s.ready
}

func (s *streamer) publisherSdpText() []byte {
	return s.serverSdpText
}

func (s *streamer) publisherSdpParsed() *sdp.SessionDescription {
	return s.serverSdpParsed
}

func (s *streamer) run() {
	for {
		ok := s.do()
		if !ok {
			break
		}

		t := time.NewTimer(_STREAMER_RETRY_INTERVAL)
		select {
		case <-s.terminate:
			break
		case <-t.C:
		}
	}

	close(s.done)
}

func (s *streamer) do() bool {
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

	conn, err := gortsplib.NewConnClient(gortsplib.ConnClientConf{
		Conn:         nconn,
		ReadTimeout:  s.p.conf.ReadTimeout,
		WriteTimeout: s.p.conf.WriteTimeout,
	})
	if err != nil {
		s.log("ERR: %s", err)
		return true
	}

	_, err = conn.Options(s.u)
	if err != nil {
		s.log("ERR: %s", err)
		return true
	}

	clientSdpParsed, _, err := conn.Describe(s.u)
	if err != nil {
		s.log("ERR: %s", err)
		return true
	}

	// create a filtered SDP that is used by the server (not by the client)
	serverSdpParsed, serverSdpText := sdpForServer(clientSdpParsed)

	s.clientSdpParsed = clientSdpParsed
	s.serverSdpText = serverSdpText
	s.serverSdpParsed = serverSdpParsed

	if s.proto == _STREAM_PROTOCOL_UDP {
		return s.runUdp(conn)
	} else {
		return s.runTcp(conn)
	}
}

func (s *streamer) runUdp(conn *gortsplib.ConnClient) bool {
	publisherIp := conn.NetConn().RemoteAddr().(*net.TCPAddr).IP

	var streamerUdpListenerPairs []streamerUdpListenerPair

	defer func() {
		for _, pair := range streamerUdpListenerPairs {
			pair.rtpl.close()
			pair.rtcpl.close()
		}
	}()

	for i, media := range s.clientSdpParsed.MediaDescriptions {
		var rtpPort int
		var rtcpPort int
		var rtpl *streamerUdpListener
		var rtcpl *streamerUdpListener
		func() {
			for {
				// choose two consecutive ports in range 65536-10000
				// rtp must be pair and rtcp odd
				rtpPort = (rand.Intn((65535-10000)/2) * 2) + 10000
				rtcpPort = rtpPort + 1

				var err error
				rtpl, err = newStreamerUdpListener(s.p, rtpPort, s, i,
					gortsplib.StreamTypeRtp, publisherIp)
				if err != nil {
					continue
				}

				rtcpl, err = newStreamerUdpListener(s.p, rtcpPort, s, i,
					gortsplib.StreamTypeRtcp, publisherIp)
				if err != nil {
					rtpl.close()
					continue
				}

				return
			}
		}()

		rtpServerPort, rtcpServerPort, _, err := conn.SetupUdp(s.u, media, rtpPort, rtcpPort)
		if err != nil {
			s.log("ERR: %s", err)
			rtpl.close()
			rtcpl.close()
			return true
		}

		rtpl.publisherPort = rtpServerPort
		rtcpl.publisherPort = rtcpServerPort

		streamerUdpListenerPairs = append(streamerUdpListenerPairs, streamerUdpListenerPair{
			rtpl:  rtpl,
			rtcpl: rtcpl,
		})
	}

	_, err := conn.Play(s.u)
	if err != nil {
		s.log("ERR: %s", err)
		return true
	}

	s.rtcpReceivers = make([]*rtcpReceiver, len(s.clientSdpParsed.MediaDescriptions))
	for trackId := range s.clientSdpParsed.MediaDescriptions {
		s.rtcpReceivers[trackId] = newRtcpReceiver()
	}

	for _, pair := range streamerUdpListenerPairs {
		pair.rtpl.start()
		pair.rtcpl.start()
	}

	sendKeepaliveTicker := time.NewTicker(_STREAMER_KEEPALIVE_INTERVAL)
	checkStreamTicker := time.NewTicker(_STREAMER_CHECK_STREAM_INTERVAL)
	receiverReportTicker := time.NewTicker(_STREAMER_RECEIVER_REPORT_INTERVAL)

	s.p.events <- programEventStreamerReady{s}

	var ret bool

outer:
	for {
		select {
		case <-s.terminate:
			ret = false
			break outer

		case <-sendKeepaliveTicker.C:
			_, err = conn.Do(&gortsplib.Request{
				Method: gortsplib.OPTIONS,
				Url: &url.URL{
					Scheme: "rtsp",
					Host:   s.u.Host,
					Path:   "/",
				},
			})
			if err != nil {
				s.log("ERR: %s", err)
				ret = true
				break outer
			}

		case <-checkStreamTicker.C:
			for trackId := range s.clientSdpParsed.MediaDescriptions {
				if time.Since(s.rtcpReceivers[trackId].lastFrameTime()) >= s.p.conf.StreamDeadAfter {
					s.log("ERR: stream is dead")
					ret = true
					break outer
				}
			}

		case <-receiverReportTicker.C:
			for trackId := range s.clientSdpParsed.MediaDescriptions {
				frame := s.rtcpReceivers[trackId].report()
				streamerUdpListenerPairs[trackId].rtcpl.writeChan <- &udpAddrBufPair{
					addr: &net.UDPAddr{
						IP:   conn.NetConn().RemoteAddr().(*net.TCPAddr).IP,
						Zone: conn.NetConn().RemoteAddr().(*net.TCPAddr).Zone,
						Port: streamerUdpListenerPairs[trackId].rtcpl.publisherPort,
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

	for _, pair := range streamerUdpListenerPairs {
		pair.rtpl.stop()
		pair.rtcpl.stop()
	}

	for trackId := range s.clientSdpParsed.MediaDescriptions {
		s.rtcpReceivers[trackId].close()
	}

	return ret
}

func (s *streamer) runTcp(conn *gortsplib.ConnClient) bool {
	for i, media := range s.clientSdpParsed.MediaDescriptions {
		_, err := conn.SetupTcp(s.u, media, i)
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

	s.rtcpReceivers = make([]*rtcpReceiver, len(s.clientSdpParsed.MediaDescriptions))
	for trackId := range s.clientSdpParsed.MediaDescriptions {
		s.rtcpReceivers[trackId] = newRtcpReceiver()
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

			trackId, streamType := gortsplib.ConvChannelToTrackIdAndStreamType(frame.Channel)

			s.rtcpReceivers[trackId].onFrame(streamType, frame.Content)
			s.p.events <- programEventStreamerFrame{s, trackId, streamType, frame.Content}
		}
	}()

	// a ticker to check the stream is not needed since there's already a deadline
	// on the RTSP reads
	receiverReportTicker := time.NewTicker(_STREAMER_RECEIVER_REPORT_INTERVAL)

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
			for trackId := range s.clientSdpParsed.MediaDescriptions {
				frame := s.rtcpReceivers[trackId].report()

				channel := gortsplib.ConvTrackIdAndStreamTypeToChannel(trackId, gortsplib.StreamTypeRtcp)

				conn.WriteFrame(&gortsplib.InterleavedFrame{
					Channel: channel,
					Content: frame,
				})
			}
		}
	}

	receiverReportTicker.Stop()

	s.p.events <- programEventStreamerNotReady{s}

	for trackId := range s.clientSdpParsed.MediaDescriptions {
		s.rtcpReceivers[trackId].close()
	}

	return ret
}

func (s *streamer) close() {
	close(s.terminate)
	<-s.done
}
