package main

import (
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"strconv"
	"strings"
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
	ur              *url.URL
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
	ur, err := url.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("'%s' is not a valid source not an RTSP url", source)
	}
	if ur.Scheme != "rtsp" {
		return nil, fmt.Errorf("'%s' is not a valid RTSP url", source)
	}
	if ur.Port() == "" {
		ur.Host += ":554"
	}
	if ur.User != nil {
		pass, _ := ur.User.Password()
		user := ur.User.Username()
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
		ur:        ur,
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
		nconn, err = net.DialTimeout("tcp", s.ur.Host, s.p.conf.ReadTimeout)
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
		NConn: nconn,
		Username: func() string {
			if s.ur.User != nil {
				return s.ur.User.Username()
			}
			return ""
		}(),
		Password: func() string {
			if s.ur.User != nil {
				pass, _ := s.ur.User.Password()
				return pass
			}
			return ""
		}(),
		ReadTimeout:  s.p.conf.ReadTimeout,
		WriteTimeout: s.p.conf.WriteTimeout,
	})
	if err != nil {
		s.log("ERR: %s", err)
		return true
	}

	res, err := conn.WriteRequest(&gortsplib.Request{
		Method: gortsplib.OPTIONS,
		Url: &url.URL{
			Scheme: "rtsp",
			Host:   s.ur.Host,
			Path:   "/",
		},
	})
	if err != nil {
		s.log("ERR: %s", err)
		return true
	}

	// OPTIONS is not available in some cameras
	if res.StatusCode != gortsplib.StatusOK && res.StatusCode != gortsplib.StatusNotFound {
		s.log("ERR: OPTIONS returned code %d (%s)", res.StatusCode, res.StatusMessage)
		return true
	}

	res, err = conn.WriteRequest(&gortsplib.Request{
		Method: gortsplib.DESCRIBE,
		Url: &url.URL{
			Scheme:   "rtsp",
			Host:     s.ur.Host,
			Path:     s.ur.Path,
			RawQuery: s.ur.RawQuery,
		},
	})
	if err != nil {
		s.log("ERR: %s", err)
		return true
	}

	if res.StatusCode != gortsplib.StatusOK {
		s.log("ERR: DESCRIBE returned code %d (%s)", res.StatusCode, res.StatusMessage)
		return true
	}

	contentType, ok := res.Header["Content-Type"]
	if !ok || len(contentType) != 1 {
		s.log("ERR: Content-Type not provided")
		return true
	}

	if contentType[0] != "application/sdp" {
		s.log("ERR: wrong Content-Type, expected application/sdp")
		return true
	}

	clientSdpParsed := &sdp.SessionDescription{}
	err = clientSdpParsed.Unmarshal(string(res.Content))
	if err != nil {
		s.log("ERR: invalid SDP: %s", err)
		return true
	}

	// create a filtered SDP that is used by the server (not by the client)
	serverSdpParsed, serverSdpText := sdpForServer(clientSdpParsed, res.Content)

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
					_TRACK_FLOW_TYPE_RTP, publisherIp)
				if err != nil {
					continue
				}

				rtcpl, err = newStreamerUdpListener(s.p, rtcpPort, s, i,
					_TRACK_FLOW_TYPE_RTCP, publisherIp)
				if err != nil {
					rtpl.close()
					continue
				}

				return
			}
		}()

		res, err := conn.WriteRequest(&gortsplib.Request{
			Method: gortsplib.SETUP,
			Url: func() *url.URL {
				control := sdpFindAttribute(media.Attributes, "control")

				// no control attribute
				if control == "" {
					return s.ur
				}

				// absolute path
				if strings.HasPrefix(control, "rtsp://") {
					ur, err := url.Parse(control)
					if err != nil {
						return s.ur
					}
					return ur
				}

				// relative path
				return &url.URL{
					Scheme: "rtsp",
					Host:   s.ur.Host,
					Path: func() string {
						ret := s.ur.Path

						if len(ret) == 0 || ret[len(ret)-1] != '/' {
							ret += "/"
						}

						control := sdpFindAttribute(media.Attributes, "control")
						if control != "" {
							ret += control
						} else {
							ret += "trackID=" + strconv.FormatInt(int64(i+1), 10)
						}

						return ret
					}(),
					RawQuery: s.ur.RawQuery,
				}
			}(),
			Header: gortsplib.Header{
				"Transport": []string{strings.Join([]string{
					"RTP/AVP/UDP",
					"unicast",
					fmt.Sprintf("client_port=%d-%d", rtpPort, rtcpPort),
				}, ";")},
			},
		})
		if err != nil {
			s.log("ERR: %s", err)
			rtpl.close()
			rtcpl.close()
			return true
		}

		if res.StatusCode != gortsplib.StatusOK {
			s.log("ERR: SETUP returned code %d (%s)", res.StatusCode, res.StatusMessage)
			rtpl.close()
			rtcpl.close()
			return true
		}

		tsRaw, ok := res.Header["Transport"]
		if !ok || len(tsRaw) != 1 {
			s.log("ERR: transport header not provided")
			rtpl.close()
			rtcpl.close()
			return true
		}

		th := gortsplib.ReadHeaderTransport(tsRaw[0])
		rtpServerPort, rtcpServerPort := th.GetPorts("server_port")
		if rtpServerPort == 0 {
			s.log("ERR: server ports not provided")
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

	res, err := conn.WriteRequest(&gortsplib.Request{
		Method: gortsplib.PLAY,
		Url: &url.URL{
			Scheme:   "rtsp",
			Host:     s.ur.Host,
			Path:     s.ur.Path,
			RawQuery: s.ur.RawQuery,
		},
	})
	if err != nil {
		s.log("ERR: %s", err)
		return true
	}

	if res.StatusCode != gortsplib.StatusOK {
		s.log("ERR: PLAY returned code %d (%s)", res.StatusCode, res.StatusMessage)
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
			_, err = conn.WriteRequest(&gortsplib.Request{
				Method: gortsplib.OPTIONS,
				Url: &url.URL{
					Scheme: "rtsp",
					Host:   s.ur.Host,
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
		interleaved := fmt.Sprintf("interleaved=%d-%d", (i * 2), (i*2)+1)

		res, err := conn.WriteRequest(&gortsplib.Request{
			Method: gortsplib.SETUP,
			Url: func() *url.URL {
				control := sdpFindAttribute(media.Attributes, "control")

				// no control attribute
				if control == "" {
					return s.ur
				}

				// absolute path
				if strings.HasPrefix(control, "rtsp://") {
					ur, err := url.Parse(control)
					if err != nil {
						return s.ur
					}
					return ur
				}

				// relative path
				return &url.URL{
					Scheme: "rtsp",
					Host:   s.ur.Host,
					Path: func() string {
						ret := s.ur.Path

						if len(ret) == 0 || ret[len(ret)-1] != '/' {
							ret += "/"
						}

						control := sdpFindAttribute(media.Attributes, "control")
						if control != "" {
							ret += control
						} else {
							ret += "trackID=" + strconv.FormatInt(int64(i+1), 10)
						}

						return ret
					}(),
					RawQuery: s.ur.RawQuery,
				}
			}(),
			Header: gortsplib.Header{
				"Transport": []string{strings.Join([]string{
					"RTP/AVP/TCP",
					"unicast",
					interleaved,
				}, ";")},
			},
		})
		if err != nil {
			s.log("ERR: %s", err)
			return true
		}

		if res.StatusCode != gortsplib.StatusOK {
			s.log("ERR: SETUP returned code %d (%s)", res.StatusCode, res.StatusMessage)
			return true
		}

		tsRaw, ok := res.Header["Transport"]
		if !ok || len(tsRaw) != 1 {
			s.log("ERR: transport header not provided")
			return true
		}

		th := gortsplib.ReadHeaderTransport(tsRaw[0])

		_, ok = th[interleaved]
		if !ok {
			s.log("ERR: transport header does not have %s (%s)", interleaved, tsRaw[0])
			return true
		}
	}

	err := conn.WriteRequestNoResponse(&gortsplib.Request{
		Method: gortsplib.PLAY,
		Url: &url.URL{
			Scheme:   "rtsp",
			Host:     s.ur.Host,
			Path:     s.ur.Path,
			RawQuery: s.ur.RawQuery,
		},
	})
	if err != nil {
		s.log("ERR: %s", err)
		return true
	}

	frame := &gortsplib.InterleavedFrame{}

outer1:
	for {
		frame.Content = s.readBuf.swap()
		frame.Content = frame.Content[:cap(frame.Content)]

		recv, err := conn.ReadInterleavedFrameOrResponse(frame)
		if err != nil {
			s.log("ERR: %s", err)
			return true
		}

		switch recvt := recv.(type) {
		case *gortsplib.Response:
			if recvt.StatusCode != gortsplib.StatusOK {
				s.log("ERR: PLAY returned code %d (%s)", recvt.StatusCode, recvt.StatusMessage)
				return true
			}
			break outer1

		case *gortsplib.InterleavedFrame:
			// ignore the frames sent before the response
		}
	}

	s.rtcpReceivers = make([]*rtcpReceiver, len(s.clientSdpParsed.MediaDescriptions))
	for trackId := range s.clientSdpParsed.MediaDescriptions {
		s.rtcpReceivers[trackId] = newRtcpReceiver()
	}

	s.p.events <- programEventStreamerReady{s}

	chanConnError := make(chan struct{})
	go func() {
		for {
			frame.Content = s.readBuf.swap()
			frame.Content = frame.Content[:cap(frame.Content)]
			err := conn.ReadInterleavedFrame(frame)
			if err != nil {
				s.log("ERR: %s", err)
				close(chanConnError)
				break
			}

			trackId, trackFlowType := interleavedChannelToTrackFlowType(frame.Channel)

			s.rtcpReceivers[trackId].onFrame(trackFlowType, frame.Content)
			s.p.events <- programEventStreamerFrame{s, trackId, trackFlowType, frame.Content}
		}
	}()

	checkStreamTicker := time.NewTicker(_STREAMER_CHECK_STREAM_INTERVAL)
	receiverReportTicker := time.NewTicker(_STREAMER_RECEIVER_REPORT_INTERVAL)

	var ret bool

outer2:
	for {
		select {
		case <-s.terminate:
			ret = false
			break outer2

		case <-chanConnError:
			ret = true
			break outer2

		case <-checkStreamTicker.C:
			for trackId := range s.clientSdpParsed.MediaDescriptions {
				if time.Since(s.rtcpReceivers[trackId].lastFrameTime()) >= s.p.conf.StreamDeadAfter {
					s.log("ERR: stream is dead")
					ret = true
					break outer2
				}
			}

		case <-receiverReportTicker.C:
			for trackId := range s.clientSdpParsed.MediaDescriptions {
				frame := s.rtcpReceivers[trackId].report()

				channel := trackFlowTypeToInterleavedChannel(trackId, _TRACK_FLOW_TYPE_RTCP)

				conn.WriteInterleavedFrame(&gortsplib.InterleavedFrame{
					Channel: channel,
					Content: frame,
				})
			}
		}
	}

	checkStreamTicker.Stop()
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
