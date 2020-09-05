package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/sdp-dirty/v3"
)

const (
	clientCheckStreamInterval    = 5 * time.Second
	clientReceiverReportInterval = 10 * time.Second
	clientTCPReadBufferSize      = 128 * 1024
	clientTCPWriteBufferSize     = 128 * 1024
	clientUDPReadBufferSize      = 2048
	clientUDPWriteBufferSize     = 128 * 1024
)

type clientDescribeReq struct {
	client   *client
	pathName string
}

type clientAnnounceReq struct {
	res       chan error
	client    *client
	pathName  string
	sdpText   []byte
	sdpParsed *sdp.SessionDescription
}

type clientSetupPlayReq struct {
	res      chan error
	client   *client
	pathName string
	trackId  int
}

type clientFrameUDPReq struct {
	addr       *net.UDPAddr
	streamType gortsplib.StreamType
	buf        []byte
}

type clientFrameTCPReq struct {
	path       *path
	trackId    int
	streamType gortsplib.StreamType
	buf        []byte
}

type udpClient struct {
	client     *client
	trackId    int
	streamType gortsplib.StreamType
}

type udpClientAddr struct {
	ip   [net.IPv6len]byte // use a fixed-size array to enable the equality operator
	port int
}

func makeUDPClientAddr(ip net.IP, port int) udpClientAddr {
	ret := udpClientAddr{
		port: port,
	}

	if len(ip) == net.IPv4len {
		copy(ret.ip[0:], []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}) // v4InV6Prefix
		copy(ret.ip[12:], ip)
	} else {
		copy(ret.ip[:], ip)
	}

	return ret
}

type clientTrack struct {
	rtpPort  int
	rtcpPort int
}

type describeRes struct {
	sdp []byte
	err error
}

type clientState int

const (
	clientStateInitial clientState = iota
	clientStateWaitDescription
	clientStatePrePlay
	clientStatePlay
	clientStatePreRecord
	clientStateRecord
)

func (cs clientState) String() string {
	switch cs {
	case clientStateInitial:
		return "Initial"

	case clientStateWaitDescription:
		return "WaitDescription"

	case clientStatePrePlay:
		return "PrePlay"

	case clientStatePlay:
		return "Play"

	case clientStatePreRecord:
		return "PreRecord"

	case clientStateRecord:
		return "Record"
	}
	return "Invalid"
}

type client struct {
	p              *program
	conn           *gortsplib.ConnServer
	state          clientState
	path           *path
	authUser       string
	authPass       string
	authHelper     *gortsplib.AuthServer
	authFailures   int
	streamProtocol gortsplib.StreamProtocol
	streamTracks   map[int]*clientTrack
	rtcpReceivers  []*gortsplib.RtcpReceiver
	describeCSeq   gortsplib.HeaderValue
	describeUrl    string

	describe  chan describeRes
	tcpFrame  chan *gortsplib.InterleavedFrame
	terminate chan struct{}
	done      chan struct{}
}

func newClient(p *program, nconn net.Conn) *client {
	c := &client{
		p: p,
		conn: gortsplib.NewConnServer(gortsplib.ConnServerConf{
			Conn:         nconn,
			ReadTimeout:  p.conf.ReadTimeout,
			WriteTimeout: p.conf.WriteTimeout,
		}),
		state:        clientStateInitial,
		streamTracks: make(map[int]*clientTrack),
		describe:     make(chan describeRes),
		tcpFrame:     make(chan *gortsplib.InterleavedFrame),
		terminate:    make(chan struct{}),
		done:         make(chan struct{}),
	}

	go c.run()
	return c
}

func (c *client) close() {
	delete(c.p.clients, c)

	atomic.AddInt64(&c.p.countClient, -1)

	switch c.state {
	case clientStatePlay:
		atomic.AddInt64(&c.p.countReader, -1)

	case clientStateRecord:
		atomic.AddInt64(&c.p.countPublisher, -1)

		if c.streamProtocol == gortsplib.StreamProtocolUDP {
			for _, track := range c.streamTracks {
				key := makeUDPClientAddr(c.ip(), track.rtpPort)
				delete(c.p.udpClientsByAddr, key)

				key = makeUDPClientAddr(c.ip(), track.rtcpPort)
				delete(c.p.udpClientsByAddr, key)
			}
		}

		c.path.onPublisherSetNotReady()
	}

	if c.path != nil && c.path.publisher == c {
		c.path.onPublisherRemove()
	}

	close(c.terminate)

	c.log("disconnected")
}

func (c *client) log(format string, args ...interface{}) {
	c.p.log("[client %s] "+format, append([]interface{}{c.conn.NetConn().RemoteAddr().String()}, args...)...)
}

func (c *client) isPublisher() {}

func (c *client) ip() net.IP {
	return c.conn.NetConn().RemoteAddr().(*net.TCPAddr).IP
}

func (c *client) zone() string {
	return c.conn.NetConn().RemoteAddr().(*net.TCPAddr).Zone
}

var errRunTerminate = errors.New("terminate")
var errRunWaitDescription = errors.New("wait description")
var errRunPlay = errors.New("play")
var errRunRecord = errors.New("record")

func (c *client) run() {
	var onConnectCmd *exec.Cmd
	if c.p.conf.RunOnConnect != "" {
		onConnectCmd = exec.Command("/bin/sh", "-c", c.p.conf.RunOnConnect)
		onConnectCmd.Stdout = os.Stdout
		onConnectCmd.Stderr = os.Stderr
		err := onConnectCmd.Start()
		if err != nil {
			c.log("ERR: %s", err)
		}
	}

	for {
		if !c.runInitial() {
			break
		}
	}

	if onConnectCmd != nil {
		onConnectCmd.Process.Signal(os.Interrupt)
		onConnectCmd.Wait()
	}

	close(c.describe)
	close(c.tcpFrame)
	close(c.done)
}

func (c *client) writeResError(cseq gortsplib.HeaderValue, code gortsplib.StatusCode, err error) {
	c.log("ERR: %s", err)

	c.conn.WriteResponse(&gortsplib.Response{
		StatusCode: code,
		Header: gortsplib.Header{
			"CSeq": cseq,
		},
	})
}

var errAuthCritical = errors.New("auth critical")
var errAuthNotCritical = errors.New("auth not critical")

func (c *client) authenticate(ips []interface{}, user string, pass string, req *gortsplib.Request) error {
	// validate ip
	err := func() error {
		if ips == nil {
			return nil
		}

		ip := c.ip()
		if !ipEqualOrInRange(ip, ips) {
			c.log("ERR: ip '%s' not allowed", ip)
			return errAuthCritical
		}

		return nil
	}()
	if err != nil {
		return err
	}

	// validate credentials
	err = func() error {
		if user == "" {
			return nil
		}

		// reset authHelper every time the credentials change
		if c.authHelper == nil || c.authUser != user || c.authPass != pass {
			c.authUser = user
			c.authPass = pass
			c.authHelper = gortsplib.NewAuthServer(user, pass, c.p.conf.authMethodsParsed)
		}

		err := c.authHelper.ValidateHeader(req.Header["Authorization"], req.Method, req.Url)
		if err != nil {
			c.authFailures += 1

			// vlc with login prompt sends 4 requests:
			// 1) without credentials
			// 2) with password but without the username
			// 3) without credentials
			// 4) with password and username
			// hence we must allow up to 3 failures
			var retErr error
			if c.authFailures > 3 {
				c.log("ERR: unauthorized: %s", err)
				retErr = errAuthCritical

			} else if c.authFailures > 1 {
				c.log("WARN: unauthorized: %s", err)
				retErr = errAuthNotCritical

			} else {
				retErr = errAuthNotCritical
			}

			c.conn.WriteResponse(&gortsplib.Response{
				StatusCode: gortsplib.StatusUnauthorized,
				Header: gortsplib.Header{
					"CSeq":             req.Header["CSeq"],
					"WWW-Authenticate": c.authHelper.GenerateHeader(),
				},
			})

			return retErr
		}

		// reset authFailures after a successful login
		c.authFailures = 0

		return nil
	}()
	if err != nil {
		return err
	}

	return nil
}

func (c *client) handleRequest(req *gortsplib.Request) error {
	c.log(string(req.Method))

	cseq, ok := req.Header["CSeq"]
	if !ok || len(cseq) != 1 {
		c.writeResError(nil, gortsplib.StatusBadRequest, fmt.Errorf("cseq missing"))
		return errRunTerminate
	}

	pathName := req.Url.Path
	if len(pathName) < 1 || pathName[0] != '/' {
		c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("path must begin with a slash"))
		return errRunTerminate
	}
	pathName = pathName[1:] // strip leading slash

	switch req.Method {
	case gortsplib.OPTIONS:
		c.conn.WriteResponse(&gortsplib.Response{
			StatusCode: gortsplib.StatusOK,
			Header: gortsplib.Header{
				"CSeq": cseq,
				"Public": gortsplib.HeaderValue{strings.Join([]string{
					string(gortsplib.DESCRIBE),
					string(gortsplib.ANNOUNCE),
					string(gortsplib.SETUP),
					string(gortsplib.PLAY),
					string(gortsplib.RECORD),
					string(gortsplib.TEARDOWN),
				}, ", ")},
			},
		})
		return nil

	case gortsplib.DESCRIBE:
		if c.state != clientStateInitial {
			c.writeResError(cseq, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, clientStateInitial))
			return errRunTerminate
		}

		confp := c.p.findConfForPathName(pathName)
		if confp == nil {
			c.writeResError(cseq, gortsplib.StatusBadRequest,
				fmt.Errorf("unable to find a valid configuration for path '%s'", pathName))
			return errRunTerminate
		}

		err := c.authenticate(confp.readIpsParsed, confp.ReadUser, confp.ReadPass, req)
		if err != nil {
			if err == errAuthCritical {
				return errRunTerminate
			}
			return nil
		}

		c.p.clientDescribe <- clientDescribeReq{c, pathName}

		c.describeCSeq = cseq
		c.describeUrl = req.Url.String()

		return errRunWaitDescription

	case gortsplib.ANNOUNCE:
		if c.state != clientStateInitial {
			c.writeResError(cseq, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, clientStateInitial))
			return errRunTerminate
		}

		if len(pathName) == 0 {
			c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("empty base path"))
			return errRunTerminate
		}

		err := checkPathName(pathName)
		if err != nil {
			c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("invalid path name: %s (%s)", err, pathName))
			return errRunTerminate
		}

		confp := c.p.findConfForPathName(pathName)
		if confp == nil {
			c.writeResError(cseq, gortsplib.StatusBadRequest,
				fmt.Errorf("unable to find a valid configuration for path '%s'", pathName))
			return errRunTerminate
		}

		err = c.authenticate(confp.publishIpsParsed, confp.PublishUser, confp.PublishPass, req)
		if err != nil {
			if err == errAuthCritical {
				return errRunTerminate
			}
			return nil
		}

		ct, ok := req.Header["Content-Type"]
		if !ok || len(ct) != 1 {
			c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("Content-Type header missing"))
			return errRunTerminate
		}

		if ct[0] != "application/sdp" {
			c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("unsupported Content-Type '%s'", ct))
			return errRunTerminate
		}

		sdpParsed := &sdp.SessionDescription{}
		err = sdpParsed.Unmarshal(req.Content)
		if err != nil {
			c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("invalid SDP: %s", err))
			return errRunTerminate
		}

		if len(sdpParsed.MediaDescriptions) == 0 {
			c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("no tracks defined"))
			return errRunTerminate
		}

		var tracks []*gortsplib.Track
		for i, media := range sdpParsed.MediaDescriptions {
			tracks = append(tracks, &gortsplib.Track{
				Id:    i,
				Media: media,
			})
		}
		sdpParsed, req.Content = sdpForServer(tracks)

		res := make(chan error)
		c.p.clientAnnounce <- clientAnnounceReq{res, c, pathName, req.Content, sdpParsed}
		err = <-res
		if err != nil {
			c.writeResError(cseq, gortsplib.StatusBadRequest, err)
			return errRunTerminate
		}

		c.conn.WriteResponse(&gortsplib.Response{
			StatusCode: gortsplib.StatusOK,
			Header: gortsplib.Header{
				"CSeq": cseq,
			},
		})
		return nil

	case gortsplib.SETUP:
		th, err := gortsplib.ReadHeaderTransport(req.Header["Transport"])
		if err != nil {
			c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("transport header: %s", err))
			return errRunTerminate
		}

		if _, ok := th["multicast"]; ok {
			c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("multicast is not supported"))
			return errRunTerminate
		}

		basePath, controlPath, err := splitPath(pathName)
		if err != nil {
			c.writeResError(cseq, gortsplib.StatusBadRequest, err)
			return errRunTerminate
		}

		switch c.state {
		// play
		case clientStateInitial, clientStatePrePlay:
			confp := c.p.findConfForPathName(basePath)
			if confp == nil {
				c.writeResError(cseq, gortsplib.StatusBadRequest,
					fmt.Errorf("unable to find a valid configuration for path '%s'", basePath))
				return errRunTerminate
			}

			err := c.authenticate(confp.readIpsParsed, confp.ReadUser, confp.ReadPass, req)
			if err != nil {
				if err == errAuthCritical {
					return errRunTerminate
				}
				return nil
			}

			if c.path != nil && basePath != c.path.name {
				c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.name, basePath))
				return errRunTerminate
			}

			if !strings.HasPrefix(controlPath, "trackID=") {
				c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("invalid control path (%s)", controlPath))
				return errRunTerminate
			}

			tmp, err := strconv.ParseInt(controlPath[len("trackID="):], 10, 64)
			if err != nil || tmp < 0 {
				c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("invalid track id (%s)", controlPath))
				return errRunTerminate
			}
			trackId := int(tmp)

			if _, ok := c.streamTracks[trackId]; ok {
				c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("track %d has already been setup", trackId))
				return errRunTerminate
			}

			// play via UDP
			if th.IsUDP() {
				if _, ok := c.p.conf.protocolsParsed[gortsplib.StreamProtocolUDP]; !ok {
					c.writeResError(cseq, gortsplib.StatusUnsupportedTransport, fmt.Errorf("UDP streaming is disabled"))
					return errRunTerminate
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolUDP {
					c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("can't receive tracks with different protocols"))
					return errRunTerminate
				}

				rtpPort, rtcpPort := th.Ports("client_port")
				if rtpPort == 0 || rtcpPort == 0 {
					c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not have valid client ports (%v)", req.Header["Transport"]))
					return errRunTerminate
				}

				res := make(chan error)
				c.p.clientSetupPlay <- clientSetupPlayReq{res, c, basePath, trackId}
				err = <-res
				if err != nil {
					c.writeResError(cseq, gortsplib.StatusBadRequest, err)
					return errRunTerminate
				}

				c.streamProtocol = gortsplib.StreamProtocolUDP
				c.streamTracks[trackId] = &clientTrack{
					rtpPort:  rtpPort,
					rtcpPort: rtcpPort,
				}

				c.conn.WriteResponse(&gortsplib.Response{
					StatusCode: gortsplib.StatusOK,
					Header: gortsplib.Header{
						"CSeq": cseq,
						"Transport": gortsplib.HeaderValue{strings.Join([]string{
							"RTP/AVP/UDP",
							"unicast",
							fmt.Sprintf("client_port=%d-%d", rtpPort, rtcpPort),
							fmt.Sprintf("server_port=%d-%d", c.p.conf.RtpPort, c.p.conf.RtcpPort),
						}, ";")},
						"Session": gortsplib.HeaderValue{"12345678"},
					},
				})
				return nil

				// play via TCP
			} else if th.IsTCP() {
				if _, ok := c.p.conf.protocolsParsed[gortsplib.StreamProtocolTCP]; !ok {
					c.writeResError(cseq, gortsplib.StatusUnsupportedTransport, fmt.Errorf("TCP streaming is disabled"))
					return errRunTerminate
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolTCP {
					c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("can't receive tracks with different protocols"))
					return errRunTerminate
				}

				res := make(chan error)
				c.p.clientSetupPlay <- clientSetupPlayReq{res, c, basePath, trackId}
				err = <-res
				if err != nil {
					c.writeResError(cseq, gortsplib.StatusBadRequest, err)
					return errRunTerminate
				}

				c.streamProtocol = gortsplib.StreamProtocolTCP
				c.streamTracks[trackId] = &clientTrack{
					rtpPort:  0,
					rtcpPort: 0,
				}

				interleaved := fmt.Sprintf("%d-%d", ((trackId) * 2), ((trackId)*2)+1)

				c.conn.WriteResponse(&gortsplib.Response{
					StatusCode: gortsplib.StatusOK,
					Header: gortsplib.Header{
						"CSeq": cseq,
						"Transport": gortsplib.HeaderValue{strings.Join([]string{
							"RTP/AVP/TCP",
							"unicast",
							fmt.Sprintf("interleaved=%s", interleaved),
						}, ";")},
						"Session": gortsplib.HeaderValue{"12345678"},
					},
				})
				return nil

			} else {
				c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not contain a valid protocol (RTP/AVP, RTP/AVP/UDP or RTP/AVP/TCP) (%s)", req.Header["Transport"]))
				return errRunTerminate
			}

		// record
		case clientStatePreRecord:
			if strings.ToLower(th.Value("mode")) != "record" {
				c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not contain mode=record"))
				return errRunTerminate
			}

			// after ANNOUNCE, c.path is already set
			if basePath != c.path.name {
				c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.name, basePath))
				return errRunTerminate
			}

			// record via UDP
			if th.IsUDP() {
				if _, ok := c.p.conf.protocolsParsed[gortsplib.StreamProtocolUDP]; !ok {
					c.writeResError(cseq, gortsplib.StatusUnsupportedTransport, fmt.Errorf("UDP streaming is disabled"))
					return errRunTerminate
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolUDP {
					c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("can't publish tracks with different protocols"))
					return errRunTerminate
				}

				rtpPort, rtcpPort := th.Ports("client_port")
				if rtpPort == 0 || rtcpPort == 0 {
					c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not have valid client ports (%s)", req.Header["Transport"]))
					return errRunTerminate
				}

				if len(c.streamTracks) >= len(c.path.publisherSdpParsed.MediaDescriptions) {
					c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("all the tracks have already been setup"))
					return errRunTerminate
				}

				c.streamProtocol = gortsplib.StreamProtocolUDP
				c.streamTracks[len(c.streamTracks)] = &clientTrack{
					rtpPort:  rtpPort,
					rtcpPort: rtcpPort,
				}

				c.conn.WriteResponse(&gortsplib.Response{
					StatusCode: gortsplib.StatusOK,
					Header: gortsplib.Header{
						"CSeq": cseq,
						"Transport": gortsplib.HeaderValue{strings.Join([]string{
							"RTP/AVP/UDP",
							"unicast",
							fmt.Sprintf("client_port=%d-%d", rtpPort, rtcpPort),
							fmt.Sprintf("server_port=%d-%d", c.p.conf.RtpPort, c.p.conf.RtcpPort),
						}, ";")},
						"Session": gortsplib.HeaderValue{"12345678"},
					},
				})
				return nil

				// record via TCP
			} else if th.IsTCP() {
				if _, ok := c.p.conf.protocolsParsed[gortsplib.StreamProtocolTCP]; !ok {
					c.writeResError(cseq, gortsplib.StatusUnsupportedTransport, fmt.Errorf("TCP streaming is disabled"))
					return errRunTerminate
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolTCP {
					c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("can't publish tracks with different protocols"))
					return errRunTerminate
				}

				interleaved := th.Value("interleaved")
				if interleaved == "" {
					c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not contain the interleaved field"))
					return errRunTerminate
				}

				expInterleaved := fmt.Sprintf("%d-%d", 0+len(c.streamTracks)*2, 1+len(c.streamTracks)*2)
				if interleaved != expInterleaved {
					c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("wrong interleaved value, expected '%s', got '%s'", expInterleaved, interleaved))
					return errRunTerminate
				}

				if len(c.streamTracks) >= len(c.path.publisherSdpParsed.MediaDescriptions) {
					c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("all the tracks have already been setup"))
					return errRunTerminate
				}

				c.streamProtocol = gortsplib.StreamProtocolTCP
				c.streamTracks[len(c.streamTracks)] = &clientTrack{
					rtpPort:  0,
					rtcpPort: 0,
				}

				c.conn.WriteResponse(&gortsplib.Response{
					StatusCode: gortsplib.StatusOK,
					Header: gortsplib.Header{
						"CSeq": cseq,
						"Transport": gortsplib.HeaderValue{strings.Join([]string{
							"RTP/AVP/TCP",
							"unicast",
							fmt.Sprintf("interleaved=%s", interleaved),
						}, ";")},
						"Session": gortsplib.HeaderValue{"12345678"},
					},
				})
				return nil

			} else {
				c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not contain a valid protocol (RTP/AVP, RTP/AVP/UDP or RTP/AVP/TCP) (%s)", req.Header["Transport"]))
				return errRunTerminate
			}

		default:
			c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("client is in state '%s'", c.state))
			return errRunTerminate
		}

	case gortsplib.PLAY:
		if c.state != clientStatePrePlay {
			c.writeResError(cseq, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, clientStatePrePlay))
			return errRunTerminate
		}

		// path can end with a slash, remove it
		pathName = strings.TrimSuffix(pathName, "/")

		if pathName != c.path.name {
			c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.name, pathName))
			return errRunTerminate
		}

		if len(c.streamTracks) == 0 {
			c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("no tracks have been setup"))
			return errRunTerminate
		}

		// write response before setting state
		// otherwise, in case of TCP connections, RTP packets could be sent
		// before the response
		c.conn.WriteResponse(&gortsplib.Response{
			StatusCode: gortsplib.StatusOK,
			Header: gortsplib.Header{
				"CSeq":    cseq,
				"Session": gortsplib.HeaderValue{"12345678"},
			},
		})

		return errRunPlay

	case gortsplib.RECORD:
		if c.state != clientStatePreRecord {
			c.writeResError(cseq, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, clientStatePreRecord))
			return errRunTerminate
		}

		// path can end with a slash, remove it
		pathName = strings.TrimSuffix(pathName, "/")

		if pathName != c.path.name {
			c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.name, pathName))
			return errRunTerminate
		}

		if len(c.streamTracks) != len(c.path.publisherSdpParsed.MediaDescriptions) {
			c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("not all tracks have been setup"))
			return errRunTerminate
		}

		c.conn.WriteResponse(&gortsplib.Response{
			StatusCode: gortsplib.StatusOK,
			Header: gortsplib.Header{
				"CSeq":    cseq,
				"Session": gortsplib.HeaderValue{"12345678"},
			},
		})

		return errRunRecord

	case gortsplib.TEARDOWN:
		// close connection silently
		return errRunTerminate

	default:
		c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("unhandled method '%s'", req.Method))
		return errRunTerminate
	}
}

func (c *client) runInitial() bool {
	readDone := make(chan error)
	go func() {
		for {
			req, err := c.conn.ReadRequest()
			if err != nil {
				readDone <- err
				break
			}

			err = c.handleRequest(req)
			if err != nil {
				readDone <- err
				break
			}
		}
	}()

	select {
	case err := <-readDone:
		switch err {
		case errRunWaitDescription:
			return c.runWaitDescription()

		case errRunPlay:
			return c.runPlay()

		case errRunRecord:
			return c.runRecord()

		default:
			c.conn.Close()
			if err != io.EOF && err != errRunTerminate {
				c.log("ERR: %s", err)
			}
			c.p.clientClose <- c
			<-c.terminate
			return false
		}

	case <-c.terminate:
		c.conn.Close()
		<-readDone
		return false
	}
}

func (c *client) runWaitDescription() bool {
	select {
	case res := <-c.describe:
		if res.err != nil {
			c.writeResError(c.describeCSeq, gortsplib.StatusNotFound, res.err)
			return true
		}

		c.conn.WriteResponse(&gortsplib.Response{
			StatusCode: gortsplib.StatusOK,
			Header: gortsplib.Header{
				"CSeq":         c.describeCSeq,
				"Content-Base": gortsplib.HeaderValue{c.describeUrl + "/"},
				"Content-Type": gortsplib.HeaderValue{"application/sdp"},
			},
			Content: res.sdp,
		})
		return true

	case <-c.terminate:
		c.conn.Close()
		return false
	}
}

func (c *client) runPlay() bool {
	// start sending frames only after sending the response to the PLAY request
	c.p.clientPlay <- c

	c.log("is receiving on path '%s', %d %s via %s", c.path.name, len(c.streamTracks), func() string {
		if len(c.streamTracks) == 1 {
			return "track"
		}
		return "tracks"
	}(), c.streamProtocol)

	var onReadCmd *exec.Cmd
	if c.path.confp.RunOnRead != "" {
		onReadCmd = exec.Command("/bin/sh", "-c", c.path.confp.RunOnRead)
		onReadCmd.Env = append(os.Environ(),
			"RTSP_SERVER_PATH="+c.path.name,
		)
		onReadCmd.Stdout = os.Stdout
		onReadCmd.Stderr = os.Stderr
		err := onReadCmd.Start()
		if err != nil {
			c.log("ERR: %s", err)
		}
	}

	if c.streamProtocol == gortsplib.StreamProtocolUDP {
		c.runPlayUDP()
	} else {
		c.runPlayTCP()
	}

	if onReadCmd != nil {
		onReadCmd.Process.Signal(os.Interrupt)
		onReadCmd.Wait()
	}

	return false
}

func (c *client) runPlayUDP() {
	readDone := make(chan error)
	go func() {
		for {
			req, err := c.conn.ReadRequest()
			if err != nil {
				readDone <- err
				break
			}

			err = c.handleRequest(req)
			if err != nil {
				readDone <- err
				break
			}
		}
	}()

	select {
	case err := <-readDone:
		c.conn.Close()
		if err != io.EOF && err != errRunTerminate {
			c.log("ERR: %s", err)
		}
		c.p.clientClose <- c
		<-c.terminate
		return

	case <-c.terminate:
		c.conn.Close()
		<-readDone
		return
	}
}

func (c *client) runPlayTCP() {
	readDone := make(chan error)
	go func() {
		frame := &gortsplib.InterleavedFrame{}
		readBuf := make([]byte, clientTCPReadBufferSize)

		for {
			frame.Content = readBuf
			frame.Content = frame.Content[:cap(frame.Content)]

			recv, err := c.conn.ReadFrameOrRequest(frame, false)
			if err != nil {
				readDone <- err
				break
			}

			switch recvt := recv.(type) {
			case *gortsplib.InterleavedFrame:
				// rtcp feedback is handled by gortsplib

			case *gortsplib.Request:
				err := c.handleRequest(recvt)
				if err != nil {
					readDone <- err
					break
				}
			}
		}
	}()

	for {
		select {
		case err := <-readDone:
			c.conn.Close()
			if err != io.EOF && err != errRunTerminate {
				c.log("ERR: %s", err)
			}
			go func() {
				for range c.tcpFrame {
				}
			}()
			c.p.clientClose <- c
			<-c.terminate
			return

		case frame := <-c.tcpFrame:
			c.conn.WriteFrame(frame)

		case <-c.terminate:
			c.conn.Close()
			<-readDone
			return
		}
	}
}

func (c *client) runRecord() bool {
	c.rtcpReceivers = make([]*gortsplib.RtcpReceiver, len(c.streamTracks))
	for trackId := range c.streamTracks {
		c.rtcpReceivers[trackId] = gortsplib.NewRtcpReceiver()
	}

	c.p.clientRecord <- c

	c.log("is publishing on path '%s', %d %s via %s", c.path.name, len(c.streamTracks), func() string {
		if len(c.streamTracks) == 1 {
			return "track"
		}
		return "tracks"
	}(), c.streamProtocol)

	var onPublishCmd *exec.Cmd
	if c.path.confp.RunOnPublish != "" {
		onPublishCmd = exec.Command("/bin/sh", "-c", c.path.confp.RunOnPublish)
		onPublishCmd.Env = append(os.Environ(),
			"RTSP_SERVER_PATH="+c.path.name,
		)
		onPublishCmd.Stdout = os.Stdout
		onPublishCmd.Stderr = os.Stderr
		err := onPublishCmd.Start()
		if err != nil {
			c.log("ERR: %s", err)
		}
	}

	if c.streamProtocol == gortsplib.StreamProtocolUDP {
		c.runRecordUDP()
	} else {
		c.runRecordTCP()
	}

	if onPublishCmd != nil {
		onPublishCmd.Process.Signal(os.Interrupt)
		onPublishCmd.Wait()
	}

	for trackId := range c.streamTracks {
		c.rtcpReceivers[trackId].Close()
	}

	return false
}

func (c *client) runRecordUDP() {
	// open the firewall by sending packets to every channel
	for _, track := range c.streamTracks {
		c.p.serverRtp.write(
			[]byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			&net.UDPAddr{
				IP:   c.ip(),
				Zone: c.zone(),
				Port: track.rtpPort,
			})

		c.p.serverRtcp.write(
			[]byte{0x80, 0xc9, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00},
			&net.UDPAddr{
				IP:   c.ip(),
				Zone: c.zone(),
				Port: track.rtcpPort,
			})
	}

	readDone := make(chan error)
	go func() {
		for {
			req, err := c.conn.ReadRequest()
			if err != nil {
				readDone <- err
				break
			}

			err = c.handleRequest(req)
			if err != nil {
				readDone <- err
				break
			}
		}
	}()

	checkStreamTicker := time.NewTicker(clientCheckStreamInterval)
	defer checkStreamTicker.Stop()

	receiverReportTicker := time.NewTicker(clientReceiverReportInterval)
	defer receiverReportTicker.Stop()

	for {
		select {
		case err := <-readDone:
			c.conn.Close()
			if err != io.EOF && err != errRunTerminate {
				c.log("ERR: %s", err)
			}
			c.p.clientClose <- c
			<-c.terminate
			return

		case <-checkStreamTicker.C:
			for trackId := range c.streamTracks {
				if time.Since(c.rtcpReceivers[trackId].LastFrameTime()) >= c.p.conf.ReadTimeout {
					c.log("ERR: no packets received recently (maybe there's a firewall/NAT)")
					c.conn.Close()
					<-readDone
					c.p.clientClose <- c
					<-c.terminate
					return
				}
			}

		case <-receiverReportTicker.C:
			for trackId := range c.streamTracks {
				frame := c.rtcpReceivers[trackId].Report()
				c.p.serverRtcp.write(frame, &net.UDPAddr{
					IP:   c.ip(),
					Zone: c.zone(),
					Port: c.streamTracks[trackId].rtcpPort,
				})
			}

		case <-c.terminate:
			c.conn.Close()
			<-readDone
			return
		}
	}
}

func (c *client) runRecordTCP() {
	frame := &gortsplib.InterleavedFrame{}
	readBuf := newMultiBuffer(3, clientTCPReadBufferSize)

	readDone := make(chan error)
	go func() {
		for {
			frame.Content = readBuf.next()
			frame.Content = frame.Content[:cap(frame.Content)]

			recv, err := c.conn.ReadFrameOrRequest(frame, true)
			if err != nil {
				readDone <- err
				break
			}

			switch recvt := recv.(type) {
			case *gortsplib.InterleavedFrame:
				if frame.TrackId >= len(c.streamTracks) {
					readDone <- fmt.Errorf("invalid track id '%d'", frame.TrackId)
					break
				}

				c.rtcpReceivers[frame.TrackId].OnFrame(frame.StreamType, frame.Content)
				c.p.clientFrameTCP <- clientFrameTCPReq{
					c.path,
					frame.TrackId,
					frame.StreamType,
					frame.Content,
				}

			case *gortsplib.Request:
				err := c.handleRequest(recvt)
				if err != nil {
					readDone <- err
					break
				}
			}
		}
	}()

	receiverReportTicker := time.NewTicker(clientReceiverReportInterval)
	defer receiverReportTicker.Stop()

	for {
		select {
		case err := <-readDone:
			c.conn.Close()
			if err != io.EOF && err != errRunTerminate {
				c.log("ERR: %s", err)
			}
			c.p.clientClose <- c
			<-c.terminate
			return

		case <-receiverReportTicker.C:
			for trackId := range c.streamTracks {
				frame := c.rtcpReceivers[trackId].Report()
				c.conn.WriteFrame(&gortsplib.InterleavedFrame{
					TrackId:    trackId,
					StreamType: gortsplib.StreamTypeRtcp,
					Content:    frame,
				})
			}

		case <-c.terminate:
			c.conn.Close()
			<-readDone
			return
		}
	}
}
