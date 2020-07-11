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
	"gortc.io/sdp"
)

const (
	_DIAL_TIMEOUT          = 10 * time.Second
	_RETRY_INTERVAL        = 5 * time.Second
	_CHECK_STREAM_INTERVAL = 6 * time.Second
	_STREAM_DEAD_AFTER     = 5 * time.Second
	_KEEPALIVE_INTERVAL    = 60 * time.Second
)

type streamerUdpListenerPair struct {
	udplRtp  *streamerUdpListener
	udplRtcp *streamerUdpListener
}

type streamer struct {
	p               *program
	path            string
	ur              *url.URL
	proto           streamProtocol
	ready           bool
	clientSdpParsed *sdp.Message
	serverSdpText   []byte
	serverSdpParsed *sdp.Message
	firstTime       bool
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
		firstTime: true,
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

func (s *streamer) publisherSdpParsed() *sdp.Message {
	return s.serverSdpParsed
}

func (s *streamer) run() {
	for {
		ok := s.do()
		if !ok {
			break
		}
	}

	close(s.done)
}

func (s *streamer) do() bool {
	if s.firstTime {
		s.firstTime = false
	} else {
		t := time.NewTimer(_RETRY_INTERVAL)
		select {
		case <-s.terminate:
			return false
		case <-t.C:
		}
	}

	s.log("initializing with protocol %s", s.proto)

	var nconn net.Conn
	var err error
	dialDone := make(chan struct{})
	go func() {
		nconn, err = net.DialTimeout("tcp", s.ur.Host, _DIAL_TIMEOUT)
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

	clientSdpParsed, err := gortsplib.SDPParse(res.Content)
	if err != nil {
		s.log("ERR: invalid SDP: %s", err)
		return true
	}

	// create a filtered SDP that is used by the server (not by the client)
	serverSdpParsed, serverSdpText := gortsplib.SDPFilter(clientSdpParsed, res.Content)

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
			pair.udplRtp.close()
			pair.udplRtcp.close()
		}
	}()

	for i, media := range s.clientSdpParsed.Medias {
		var rtpPort int
		var rtcpPort int
		var udplRtp *streamerUdpListener
		var udplRtcp *streamerUdpListener
		func() {
			for {
				// choose two consecutive ports in range 65536-10000
				// rtp must be pair and rtcp odd
				rtpPort = (rand.Intn((65535-10000)/2) * 2) + 10000
				rtcpPort = rtpPort + 1

				var err error
				udplRtp, err = newStreamerUdpListener(s.p, rtpPort, s, i,
					_TRACK_FLOW_TYPE_RTP, publisherIp)
				if err != nil {
					continue
				}

				udplRtcp, err = newStreamerUdpListener(s.p, rtcpPort, s, i,
					_TRACK_FLOW_TYPE_RTCP, publisherIp)
				if err != nil {
					udplRtp.close()
					continue
				}

				return
			}
		}()

		res, err := conn.WriteRequest(&gortsplib.Request{
			Method: gortsplib.SETUP,
			Url: func() *url.URL {
				control := media.Attributes.Value("control")

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

						control := media.Attributes.Value("control")
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
			udplRtp.close()
			udplRtcp.close()
			return true
		}

		if res.StatusCode != gortsplib.StatusOK {
			s.log("ERR: SETUP returned code %d (%s)", res.StatusCode, res.StatusMessage)
			udplRtp.close()
			udplRtcp.close()
			return true
		}

		tsRaw, ok := res.Header["Transport"]
		if !ok || len(tsRaw) != 1 {
			s.log("ERR: transport header not provided")
			udplRtp.close()
			udplRtcp.close()
			return true
		}

		th := gortsplib.ReadHeaderTransport(tsRaw[0])
		rtpServerPort, rtcpServerPort := th.GetPorts("server_port")
		if rtpServerPort == 0 {
			s.log("ERR: server ports not provided")
			udplRtp.close()
			udplRtcp.close()
			return true
		}

		udplRtp.publisherPort = rtpServerPort
		udplRtcp.publisherPort = rtcpServerPort

		streamerUdpListenerPairs = append(streamerUdpListenerPairs, streamerUdpListenerPair{
			udplRtp:  udplRtp,
			udplRtcp: udplRtcp,
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

	for _, pair := range streamerUdpListenerPairs {
		pair.udplRtp.start()
		pair.udplRtcp.start()
	}

	tickerSendKeepalive := time.NewTicker(_KEEPALIVE_INTERVAL)
	defer tickerSendKeepalive.Stop()

	tickerCheckStream := time.NewTicker(_CHECK_STREAM_INTERVAL)
	defer tickerCheckStream.Stop()

	s.p.events <- programEventStreamerReady{s}

	defer func() {
		s.p.events <- programEventStreamerNotReady{s}
	}()

	for {
		select {
		case <-s.terminate:
			return false

		case <-tickerSendKeepalive.C:
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
				return true
			}

		case <-tickerCheckStream.C:
			lastFrameTime := time.Time{}

			for _, pair := range streamerUdpListenerPairs {
				lft := pair.udplRtp.lastFrameTime
				if lft.After(lastFrameTime) {
					lastFrameTime = lft
				}

				lft = pair.udplRtcp.lastFrameTime
				if lft.After(lastFrameTime) {
					lastFrameTime = lft
				}
			}

			if time.Since(lastFrameTime) >= _STREAM_DEAD_AFTER {
				s.log("ERR: stream is dead")
				return true
			}
		}
	}
}

func (s *streamer) runTcp(conn *gortsplib.ConnClient) bool {
	for i, media := range s.clientSdpParsed.Medias {
		interleaved := fmt.Sprintf("interleaved=%d-%d", (i * 2), (i*2)+1)

		res, err := conn.WriteRequest(&gortsplib.Request{
			Method: gortsplib.SETUP,
			Url: func() *url.URL {
				control := media.Attributes.Value("control")

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

						control := media.Attributes.Value("control")
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

outer:
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
			break outer

		case *gortsplib.InterleavedFrame:
			// ignore the frames sent before the response
		}
	}

	s.p.events <- programEventStreamerReady{s}

	defer func() {
		s.p.events <- programEventStreamerNotReady{s}
	}()

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

			s.p.events <- programEventStreamerFrame{s, trackId, trackFlowType, frame.Content}
		}
	}()

	select {
	case <-s.terminate:
		return false
	case <-chanConnError:
		return true
	}
}

func (s *streamer) close() {
	close(s.terminate)
	<-s.done
}
