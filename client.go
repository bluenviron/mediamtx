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
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/sdp/v3"
)

const (
	clientCheckStreamInterval    = 5 * time.Second
	clientReceiverReportInterval = 10 * time.Second
	clientTcpReadBufferSize      = 128 * 1024
	clientTcpWriteBufferSize     = 128 * 1024
	clientUdpReadBufferSize      = 2048
	clientUdpWriteBufferSize     = 128 * 1024
)

type describeRes struct {
	sdp []byte
	err error
}

type clientTrack struct {
	rtpPort  int
	rtcpPort int
}

type clientEvent interface {
	isServerClientEvent()
}

type clientEventFrameTcp struct {
	frame *gortsplib.InterleavedFrame
}

func (clientEventFrameTcp) isServerClientEvent() {}

type clientState int

const (
	clientStateInitial clientState = iota
	clientStateWaitingDescription
	clientStateAnnounce
	clientStatePrePlay
	clientStatePlay
	clientStatePreRecord
	clientStateRecord
)

func (cs clientState) String() string {
	switch cs {
	case clientStateInitial:
		return "INITIAL"

	case clientStateAnnounce:
		return "ANNOUNCE"

	case clientStatePrePlay:
		return "PRE_PLAY"

	case clientStatePlay:
		return "PLAY"

	case clientStatePreRecord:
		return "PRE_RECORD"

	case clientStateRecord:
		return "RECORD"
	}
	return "UNKNOWN"
}

type client struct {
	p              *program
	conn           *gortsplib.ConnServer
	state          clientState
	pathId         string
	authUser       string
	authPass       string
	authHelper     *gortsplib.AuthServer
	authFailures   int
	streamProtocol gortsplib.StreamProtocol
	streamTracks   map[int]*clientTrack
	rtcpReceivers  []*gortsplib.RtcpReceiver
	readBuf        *doubleBuffer
	writeBuf       *doubleBuffer

	describeRes chan describeRes
	events      chan clientEvent // only if state = Play and gortsplib.StreamProtocol = TCP
	done        chan struct{}
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
		readBuf:      newDoubleBuffer(clientTcpReadBufferSize),
		done:         make(chan struct{}),
	}

	go c.run()
	return c
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

outer:
	for {
		req, err := c.conn.ReadRequest()
		if err != nil {
			if err != io.EOF {
				c.log("ERR: %s", err)
			}
			break outer
		}

		ok := c.handleRequest(req)
		if !ok {
			break outer
		}
	}

	done := make(chan struct{})
	c.p.events <- programEventClientClose{done, c}
	<-done

	c.conn.NetConn().Close() // close socket in case it has not been closed yet

	if onConnectCmd != nil {
		onConnectCmd.Process.Signal(os.Interrupt)
		onConnectCmd.Wait()
	}

	close(c.done) // close() never blocks
}

func (c *client) writeResError(req *gortsplib.Request, code gortsplib.StatusCode, err error) {
	c.log("ERR: %s", err)

	header := gortsplib.Header{}
	if cseq, ok := req.Header["CSeq"]; ok && len(cseq) == 1 {
		header["CSeq"] = cseq
	}

	c.conn.WriteResponse(&gortsplib.Response{
		StatusCode: code,
		Header:     header,
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

func (c *client) handleRequest(req *gortsplib.Request) bool {
	c.log(string(req.Method))

	cseq, ok := req.Header["CSeq"]
	if !ok || len(cseq) != 1 {
		c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("cseq missing"))
		return false
	}

	path := req.Url.Path
	if len(path) < 1 || path[0] != '/' {
		c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path must begin with a slash"))
		return false
	}
	path = path[1:] // strip leading slash

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
		return true

	case gortsplib.DESCRIBE:
		if c.state != clientStateInitial {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, clientStateInitial))
			return false
		}

		confp := c.p.findConfForPath(path)
		if confp == nil {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("unable to find a valid configuration for path '%s'", path))
			return false
		}

		err := c.authenticate(confp.readIpsParsed, confp.ReadUser, confp.ReadPass, req)
		if err != nil {
			if err == errAuthCritical {
				return false
			}
			return true
		}

		c.describeRes = make(chan describeRes)
		c.p.events <- programEventClientDescribe{c, path}
		describeRes := <-c.describeRes
		if describeRes.err != nil {
			c.writeResError(req, gortsplib.StatusNotFound, describeRes.err)
			return false
		}

		c.conn.WriteResponse(&gortsplib.Response{
			StatusCode: gortsplib.StatusOK,
			Header: gortsplib.Header{
				"CSeq":         cseq,
				"Content-Base": gortsplib.HeaderValue{req.Url.String() + "/"},
				"Content-Type": gortsplib.HeaderValue{"application/sdp"},
			},
			Content: describeRes.sdp,
		})
		return true

	case gortsplib.ANNOUNCE:
		if c.state != clientStateInitial {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, clientStateInitial))
			return false
		}

		if len(path) == 0 {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("empty base path"))
			return false
		}

		if strings.Index(path, "/") >= 0 {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("slashes in the path are not supported (%s)", path))
			return false
		}

		confp := c.p.findConfForPath(path)
		if confp == nil {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("unable to find a valid configuration for path '%s'", path))
			return false
		}

		err := c.authenticate(confp.publishIpsParsed, confp.PublishUser, confp.PublishPass, req)
		if err != nil {
			if err == errAuthCritical {
				return false
			}
			return true
		}

		ct, ok := req.Header["Content-Type"]
		if !ok || len(ct) != 1 {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("Content-Type header missing"))
			return false
		}

		if ct[0] != "application/sdp" {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("unsupported Content-Type '%s'", ct))
			return false
		}

		sdpParsed := &sdp.SessionDescription{}
		err = sdpParsed.Unmarshal(req.Content)
		if err != nil {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("invalid SDP: %s", err))
			return false
		}

		if len(sdpParsed.MediaDescriptions) == 0 {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("no tracks defined"))
			return false
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
		c.p.events <- programEventClientAnnounce{res, c, path, req.Content, sdpParsed}
		err = <-res
		if err != nil {
			c.writeResError(req, gortsplib.StatusBadRequest, err)
			return false
		}

		c.conn.WriteResponse(&gortsplib.Response{
			StatusCode: gortsplib.StatusOK,
			Header: gortsplib.Header{
				"CSeq": cseq,
			},
		})
		return true

	case gortsplib.SETUP:
		th, err := gortsplib.ReadHeaderTransport(req.Header["Transport"])
		if err != nil {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header: %s", err))
			return false
		}

		if _, ok := th["multicast"]; ok {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("multicast is not supported"))
			return false
		}

		basePath, controlPath, err := splitPath(path)
		if err != nil {
			c.writeResError(req, gortsplib.StatusBadRequest, err)
			return false
		}

		switch c.state {
		// play
		case clientStateInitial, clientStatePrePlay:
			confp := c.p.findConfForPath(basePath)
			if confp == nil {
				c.writeResError(req, gortsplib.StatusBadRequest,
					fmt.Errorf("unable to find a valid configuration for path '%s'", basePath))
				return false
			}

			err := c.authenticate(confp.readIpsParsed, confp.ReadUser, confp.ReadPass, req)
			if err != nil {
				if err == errAuthCritical {
					return false
				}
				return true
			}

			if c.pathId != "" && basePath != c.pathId {
				c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.pathId, basePath))
				return false
			}

			if !strings.HasPrefix(controlPath, "trackID=") {
				c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("invalid control path (%s)", controlPath))
				return false
			}

			tmp, err := strconv.ParseInt(controlPath[len("trackID="):], 10, 64)
			if err != nil || tmp < 0 {
				c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("invalid track id (%s)", controlPath))
				return false
			}
			trackId := int(tmp)

			if _, ok := c.streamTracks[trackId]; ok {
				c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("track %d has already been setup", trackId))
				return false
			}

			// play via UDP
			if func() bool {
				_, ok := th["RTP/AVP"]
				if ok {
					return true
				}
				_, ok = th["RTP/AVP/UDP"]
				if ok {
					return true
				}
				return false
			}() {
				if _, ok := c.p.conf.protocolsParsed[gortsplib.StreamProtocolUdp]; !ok {
					c.writeResError(req, gortsplib.StatusUnsupportedTransport, fmt.Errorf("UDP streaming is disabled"))
					return false
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolUdp {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("can't receive tracks with different protocols"))
					return false
				}

				rtpPort, rtcpPort := th.Ports("client_port")
				if rtpPort == 0 || rtcpPort == 0 {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not have valid client ports (%v)", req.Header["Transport"]))
					return false
				}

				res := make(chan error)
				c.p.events <- programEventClientSetupPlay{res, c, basePath, trackId}
				err = <-res
				if err != nil {
					c.writeResError(req, gortsplib.StatusBadRequest, err)
					return false
				}

				c.streamProtocol = gortsplib.StreamProtocolUdp
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
				return true

				// play via TCP
			} else if _, ok := th["RTP/AVP/TCP"]; ok {
				if _, ok := c.p.conf.protocolsParsed[gortsplib.StreamProtocolTcp]; !ok {
					c.writeResError(req, gortsplib.StatusUnsupportedTransport, fmt.Errorf("TCP streaming is disabled"))
					return false
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolTcp {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("can't receive tracks with different protocols"))
					return false
				}

				res := make(chan error)
				c.p.events <- programEventClientSetupPlay{res, c, basePath, trackId}
				err = <-res
				if err != nil {
					c.writeResError(req, gortsplib.StatusBadRequest, err)
					return false
				}

				c.streamProtocol = gortsplib.StreamProtocolTcp
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
				return true

			} else {
				c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not contain a valid protocol (RTP/AVP, RTP/AVP/UDP or RTP/AVP/TCP) (%s)", req.Header["Transport"]))
				return false
			}

		// record
		case clientStateAnnounce, clientStatePreRecord:
			if strings.ToLower(th.Value("mode")) != "record" {
				c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not contain mode=record"))
				return false
			}

			// after ANNOUNCE, c.pathId is already set
			if basePath != c.pathId {
				c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.pathId, basePath))
				return false
			}

			// record via UDP
			if func() bool {
				_, ok := th["RTP/AVP"]
				if ok {
					return true
				}
				_, ok = th["RTP/AVP/UDP"]
				if ok {
					return true
				}
				return false
			}() {
				if _, ok := c.p.conf.protocolsParsed[gortsplib.StreamProtocolUdp]; !ok {
					c.writeResError(req, gortsplib.StatusUnsupportedTransport, fmt.Errorf("UDP streaming is disabled"))
					return false
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolUdp {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("can't publish tracks with different protocols"))
					return false
				}

				rtpPort, rtcpPort := th.Ports("client_port")
				if rtpPort == 0 || rtcpPort == 0 {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not have valid client ports (%s)", req.Header["Transport"]))
					return false
				}

				if len(c.streamTracks) >= len(c.p.paths[c.pathId].publisherSdpParsed.MediaDescriptions) {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("all the tracks have already been setup"))
					return false
				}

				res := make(chan error)
				c.p.events <- programEventClientSetupRecord{res, c}
				err := <-res
				if err != nil {
					c.writeResError(req, gortsplib.StatusBadRequest, err)
					return false
				}

				c.streamProtocol = gortsplib.StreamProtocolUdp
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
				return true

				// record via TCP
			} else if _, ok := th["RTP/AVP/TCP"]; ok {
				if _, ok := c.p.conf.protocolsParsed[gortsplib.StreamProtocolTcp]; !ok {
					c.writeResError(req, gortsplib.StatusUnsupportedTransport, fmt.Errorf("TCP streaming is disabled"))
					return false
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolTcp {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("can't publish tracks with different protocols"))
					return false
				}

				interleaved := th.Value("interleaved")
				if interleaved == "" {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not contain the interleaved field"))
					return false
				}

				expInterleaved := fmt.Sprintf("%d-%d", 0+len(c.streamTracks)*2, 1+len(c.streamTracks)*2)
				if interleaved != expInterleaved {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("wrong interleaved value, expected '%s', got '%s'", expInterleaved, interleaved))
					return false
				}

				if len(c.streamTracks) >= len(c.p.paths[c.pathId].publisherSdpParsed.MediaDescriptions) {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("all the tracks have already been setup"))
					return false
				}

				res := make(chan error)
				c.p.events <- programEventClientSetupRecord{res, c}
				err := <-res
				if err != nil {
					c.writeResError(req, gortsplib.StatusBadRequest, err)
					return false
				}

				c.streamProtocol = gortsplib.StreamProtocolTcp
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
				return true

			} else {
				c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not contain a valid protocol (RTP/AVP, RTP/AVP/UDP or RTP/AVP/TCP) (%s)", req.Header["Transport"]))
				return false
			}

		default:
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("client is in state '%s'", c.state))
			return false
		}

	case gortsplib.PLAY:
		if c.state != clientStatePrePlay {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, clientStatePrePlay))
			return false
		}

		// path can end with a slash, remove it
		path = strings.TrimSuffix(path, "/")

		if path != c.pathId {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.pathId, path))
			return false
		}

		// check publisher existence
		res := make(chan error)
		c.p.events <- programEventClientPlay1{res, c}
		err := <-res
		if err != nil {
			c.writeResError(req, gortsplib.StatusBadRequest, err)
			return false
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

		c.runPlay(path)
		return false

	case gortsplib.RECORD:
		if c.state != clientStatePreRecord {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, clientStatePreRecord))
			return false
		}

		// path can end with a slash, remove it
		path = strings.TrimSuffix(path, "/")

		if path != c.pathId {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.pathId, path))
			return false
		}

		if len(c.streamTracks) != len(c.p.paths[c.pathId].publisherSdpParsed.MediaDescriptions) {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("not all tracks have been setup"))
			return false
		}

		c.conn.WriteResponse(&gortsplib.Response{
			StatusCode: gortsplib.StatusOK,
			Header: gortsplib.Header{
				"CSeq":    cseq,
				"Session": gortsplib.HeaderValue{"12345678"},
			},
		})

		c.runRecord(path)
		return false

	case gortsplib.TEARDOWN:
		// close connection silently
		return false

	default:
		c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("unhandled method '%s'", req.Method))
		return false
	}
}

func (c *client) runPlay(path string) {
	confp := c.p.findConfForPath(path)

	if c.streamProtocol == gortsplib.StreamProtocolTcp {
		c.writeBuf = newDoubleBuffer(clientTcpWriteBufferSize)
		c.events = make(chan clientEvent)
	}

	// start sending frames only after sending the response to the PLAY request
	done := make(chan struct{})
	c.p.events <- programEventClientPlay2{done, c}
	<-done

	c.log("is receiving on path '%s', %d %s via %s", c.pathId, len(c.streamTracks), func() string {
		if len(c.streamTracks) == 1 {
			return "track"
		}
		return "tracks"
	}(), c.streamProtocol)

	var onReadCmd *exec.Cmd
	if confp.RunOnRead != "" {
		onReadCmd = exec.Command("/bin/sh", "-c", confp.RunOnRead)
		onReadCmd.Env = append(os.Environ(),
			"RTSP_SERVER_PATH="+path,
		)
		onReadCmd.Stdout = os.Stdout
		onReadCmd.Stderr = os.Stderr
		err := onReadCmd.Start()
		if err != nil {
			c.log("ERR: %s", err)
		}
	}

	if c.streamProtocol == gortsplib.StreamProtocolUdp {
		for {
			req, err := c.conn.ReadRequest()
			if err != nil {
				if err != io.EOF {
					c.log("ERR: %s", err)
				}
				break
			}

			ok := c.handleRequest(req)
			if !ok {
				break
			}
		}

		done := make(chan struct{})
		c.p.events <- programEventClientPlayStop{done, c}
		<-done

	} else {
		readDone := make(chan error)
		go func() {
			buf := make([]byte, 2048)

			for {
				_, err := c.conn.NetConn().Read(buf)
				if err != nil {
					readDone <- err
					break
				}
			}
		}()

	outer:
		for {
			select {
			case err := <-readDone:
				if err != io.EOF {
					c.log("ERR: %s", err)
				}
				break outer

			case rawEvt := <-c.events:
				switch evt := rawEvt.(type) {
				case clientEventFrameTcp:
					c.conn.WriteFrame(evt.frame)
				}
			}
		}

		go func() {
			for range c.events {
			}
		}()

		done := make(chan struct{})
		c.p.events <- programEventClientPlayStop{done, c}
		<-done

		close(c.events)
	}

	if onReadCmd != nil {
		onReadCmd.Process.Signal(os.Interrupt)
		onReadCmd.Wait()
	}
}

func (c *client) runRecord(path string) {
	confp := c.p.findConfForPath(path)

	c.rtcpReceivers = make([]*gortsplib.RtcpReceiver, len(c.streamTracks))
	for trackId := range c.streamTracks {
		c.rtcpReceivers[trackId] = gortsplib.NewRtcpReceiver()
	}

	done := make(chan struct{})
	c.p.events <- programEventClientRecord{done, c}
	<-done

	c.log("is publishing on path '%s', %d %s via %s", c.pathId, len(c.streamTracks), func() string {
		if len(c.streamTracks) == 1 {
			return "track"
		}
		return "tracks"
	}(), c.streamProtocol)

	var onPublishCmd *exec.Cmd
	if confp.RunOnPublish != "" {
		onPublishCmd = exec.Command("/bin/sh", "-c", confp.RunOnPublish)
		onPublishCmd.Env = append(os.Environ(),
			"RTSP_SERVER_PATH="+path,
		)
		onPublishCmd.Stdout = os.Stdout
		onPublishCmd.Stderr = os.Stderr
		err := onPublishCmd.Start()
		if err != nil {
			c.log("ERR: %s", err)
		}
	}

	if c.streamProtocol == gortsplib.StreamProtocolUdp {
		readDone := make(chan error)
		go func() {
			for {
				req, err := c.conn.ReadRequest()
				if err != nil {
					readDone <- err
					break
				}

				ok := c.handleRequest(req)
				if !ok {
					readDone <- nil
					break
				}
			}
		}()

		checkStreamTicker := time.NewTicker(clientCheckStreamInterval)
		receiverReportTicker := time.NewTicker(clientReceiverReportInterval)

	outer2:
		for {
			select {
			case err := <-readDone:
				if err != nil && err != io.EOF {
					c.log("ERR: %s", err)
				}
				break outer2

			case <-checkStreamTicker.C:
				for trackId := range c.streamTracks {
					if time.Since(c.rtcpReceivers[trackId].LastFrameTime()) >= c.p.conf.ReadTimeout {
						c.log("ERR: stream is dead")
						c.conn.NetConn().Close()
						<-readDone
						break outer2
					}
				}

			case <-receiverReportTicker.C:
				for trackId := range c.streamTracks {
					frame := c.rtcpReceivers[trackId].Report()
					c.p.serverRtcp.writeChan <- &udpAddrBufPair{
						addr: &net.UDPAddr{
							IP:   c.ip(),
							Zone: c.zone(),
							Port: c.streamTracks[trackId].rtcpPort,
						},
						buf: frame,
					}
				}
			}
		}

		checkStreamTicker.Stop()
		receiverReportTicker.Stop()

	} else {
		frame := &gortsplib.InterleavedFrame{}

		readDone := make(chan error)
		go func() {
			for {
				frame.Content = c.readBuf.swap()
				frame.Content = frame.Content[:cap(frame.Content)]

				recv, err := c.conn.ReadFrameOrRequest(frame)
				if err != nil {
					readDone <- err
					break
				}

				switch recvt := recv.(type) {
				case *gortsplib.InterleavedFrame:
					if frame.TrackId >= len(c.streamTracks) {
						c.log("ERR: invalid track id '%d'", frame.TrackId)
						readDone <- nil
						break
					}

					c.rtcpReceivers[frame.TrackId].OnFrame(frame.StreamType, frame.Content)
					c.p.events <- programEventClientFrameTcp{
						c.pathId,
						frame.TrackId,
						frame.StreamType,
						frame.Content,
					}

				case *gortsplib.Request:
					ok := c.handleRequest(recvt)
					if !ok {
						readDone <- nil
						break
					}
				}
			}
		}()

		receiverReportTicker := time.NewTicker(clientReceiverReportInterval)

	outer1:
		for {
			select {
			case err := <-readDone:
				if err != nil && err != io.EOF {
					c.log("ERR: %s", err)
				}
				break outer1

			case <-receiverReportTicker.C:
				for trackId := range c.streamTracks {
					frame := c.rtcpReceivers[trackId].Report()
					c.conn.WriteFrame(&gortsplib.InterleavedFrame{
						TrackId:    trackId,
						StreamType: gortsplib.StreamTypeRtcp,
						Content:    frame,
					})
				}
			}
		}

		receiverReportTicker.Stop()
	}

	done = make(chan struct{})
	c.p.events <- programEventClientRecordStop{done, c}
	<-done

	for trackId := range c.streamTracks {
		c.rtcpReceivers[trackId].Close()
	}

	if onPublishCmd != nil {
		onPublishCmd.Process.Signal(os.Interrupt)
		onPublishCmd.Wait()
	}
}
