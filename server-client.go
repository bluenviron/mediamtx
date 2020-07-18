package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/pion/sdp"
)

const (
	clientCheckStreamInterval    = 5 * time.Second
	clientReceiverReportInterval = 10 * time.Second
)

type serverClientEvent interface {
	isServerClientEvent()
}

type serverClientEventFrameTcp struct {
	frame *gortsplib.InterleavedFrame
}

func (serverClientEventFrameTcp) isServerClientEvent() {}

type serverClientState int

const (
	clientStateStarting serverClientState = iota
	clientStateAnnounce
	clientStatePrePlay
	clientStatePlay
	clientStatePreRecord
	clientStateRecord
)

func (cs serverClientState) String() string {
	switch cs {
	case clientStateStarting:
		return "STARTING"

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

type serverClient struct {
	p               *program
	conn            *gortsplib.ConnServer
	state           serverClientState
	path            string
	authUser        string
	authPass        string
	authHelper      *gortsplib.AuthServer
	authFailures    int
	streamSdpText   []byte                  // only if publisher
	streamSdpParsed *sdp.SessionDescription // only if publisher
	streamProtocol  streamProtocol
	streamTracks    []*track
	RtcpReceivers   []*gortsplib.RtcpReceiver
	readBuf         *doubleBuffer
	writeBuf        *doubleBuffer

	events chan serverClientEvent // only if state = Play and streamProtocol = TCP
	done   chan struct{}
}

func newServerClient(p *program, nconn net.Conn) *serverClient {
	c := &serverClient{
		p: p,
		conn: gortsplib.NewConnServer(gortsplib.ConnServerConf{
			Conn:         nconn,
			ReadTimeout:  p.conf.ReadTimeout,
			WriteTimeout: p.conf.WriteTimeout,
		}),
		state:   clientStateStarting,
		readBuf: newDoubleBuffer(512 * 1024),
		done:    make(chan struct{}),
	}

	go c.run()
	return c
}

func (c *serverClient) log(format string, args ...interface{}) {
	c.p.log("[client %s] "+format, append([]interface{}{c.conn.NetConn().RemoteAddr().String()}, args...)...)
}

func (c *serverClient) ip() net.IP {
	return c.conn.NetConn().RemoteAddr().(*net.TCPAddr).IP
}

func (c *serverClient) zone() string {
	return c.conn.NetConn().RemoteAddr().(*net.TCPAddr).Zone
}

func (c *serverClient) publisherIsReady() bool {
	return c.state == clientStateRecord
}

func (c *serverClient) publisherSdpText() []byte {
	return c.streamSdpText
}

func (c *serverClient) publisherSdpParsed() *sdp.SessionDescription {
	return c.streamSdpParsed
}

func (c *serverClient) run() {
	var runOnConnectCmd *exec.Cmd
	if c.p.conf.RunOnConnect != "" {
		runOnConnectCmd = exec.Command("/bin/sh", "-c", c.p.conf.RunOnConnect)
		runOnConnectCmd.Stdout = os.Stdout
		runOnConnectCmd.Stderr = os.Stderr
		err := runOnConnectCmd.Start()
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

	if runOnConnectCmd != nil {
		runOnConnectCmd.Process.Signal(os.Interrupt)
		runOnConnectCmd.Wait()
	}

	close(c.done) // close() never blocks
}

func (c *serverClient) close() {
	c.conn.NetConn().Close()
	<-c.done
}

func (c *serverClient) writeResError(req *gortsplib.Request, code gortsplib.StatusCode, err error) {
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

func (c *serverClient) findConfForPath(path string) *ConfPath {
	if pconf, ok := c.p.conf.Paths[path]; ok {
		return pconf
	}

	if pconf, ok := c.p.conf.Paths["all"]; ok {
		return pconf
	}

	return nil
}

var errAuthCritical = errors.New("auth critical")
var errAuthNotCritical = errors.New("auth not critical")

func (c *serverClient) authenticate(ips []interface{}, user string, pass string, req *gortsplib.Request) error {
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

func (c *serverClient) handleRequest(req *gortsplib.Request) bool {
	c.log(string(req.Method))

	cseq, ok := req.Header["CSeq"]
	if !ok || len(cseq) != 1 {
		c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("cseq missing"))
		return false
	}

	path := func() string {
		ret := req.Url.Path

		// remove leading slash
		if len(ret) > 1 {
			ret = ret[1:]
		}

		// strip any subpath
		if n := strings.Index(ret, "/"); n >= 0 {
			ret = ret[:n]
		}

		return ret
	}()

	switch req.Method {
	case gortsplib.OPTIONS:
		// do not check state, since OPTIONS can be requested
		// in any state

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
		if c.state != clientStateStarting {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, clientStateStarting))
			return false
		}

		pconf := c.findConfForPath(path)
		if pconf == nil {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("unable to find a valid configuration for path '%s'", path))
			return false
		}

		err := c.authenticate(pconf.readIpsParsed, pconf.ReadUser, pconf.ReadPass, req)
		if err != nil {
			if err == errAuthCritical {
				return false
			}
			return true
		}

		res := make(chan []byte)
		c.p.events <- programEventClientDescribe{path, res}
		sdp := <-res
		if sdp == nil {
			c.writeResError(req, gortsplib.StatusNotFound, fmt.Errorf("no one is publishing on path '%s'", path))
			return false
		}

		c.conn.WriteResponse(&gortsplib.Response{
			StatusCode: gortsplib.StatusOK,
			Header: gortsplib.Header{
				"CSeq":         cseq,
				"Content-Base": gortsplib.HeaderValue{req.Url.String() + "/"},
				"Content-Type": gortsplib.HeaderValue{"application/sdp"},
			},
			Content: sdp,
		})
		return true

	case gortsplib.ANNOUNCE:
		if c.state != clientStateStarting {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, clientStateStarting))
			return false
		}

		if len(path) == 0 {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path can't be empty"))
			return false
		}

		pconf := c.findConfForPath(path)
		if pconf == nil {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("unable to find a valid configuration for path '%s'", path))
			return false
		}

		err := c.authenticate(pconf.publishIpsParsed, pconf.PublishUser, pconf.PublishPass, req)
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
		err = sdpParsed.Unmarshal(string(req.Content))
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
		c.p.events <- programEventClientAnnounce{res, c, path}
		err = <-res
		if err != nil {
			c.writeResError(req, gortsplib.StatusBadRequest, err)
			return false
		}

		c.streamSdpText = req.Content
		c.streamSdpParsed = sdpParsed

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

		switch c.state {
		// play
		case clientStateStarting, clientStatePrePlay:
			pconf := c.findConfForPath(path)
			if pconf == nil {
				c.writeResError(req, gortsplib.StatusBadRequest,
					fmt.Errorf("unable to find a valid configuration for path '%s'", path))
				return false
			}

			err := c.authenticate(pconf.readIpsParsed, pconf.ReadUser, pconf.ReadPass, req)
			if err != nil {
				if err == errAuthCritical {
					return false
				}
				return true
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
				if _, ok := c.p.conf.protocolsParsed[streamProtocolUdp]; !ok {
					c.writeResError(req, gortsplib.StatusUnsupportedTransport, fmt.Errorf("UDP streaming is disabled"))
					return false
				}

				rtpPort, rtcpPort := th.Ports("client_port")
				if rtpPort == 0 || rtcpPort == 0 {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not have valid client ports (%v)", req.Header["Transport"]))
					return false
				}

				if c.path != "" && path != c.path {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path has changed"))
					return false
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != streamProtocolUdp {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("can't receive tracks with different protocols"))
					return false
				}

				res := make(chan error)
				c.p.events <- programEventClientSetupPlay{res, c, path, streamProtocolUdp, rtpPort, rtcpPort}
				err = <-res
				if err != nil {
					c.writeResError(req, gortsplib.StatusBadRequest, err)
					return false
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
				if _, ok := c.p.conf.protocolsParsed[streamProtocolTcp]; !ok {
					c.writeResError(req, gortsplib.StatusUnsupportedTransport, fmt.Errorf("TCP streaming is disabled"))
					return false
				}

				if c.path != "" && path != c.path {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path has changed"))
					return false
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != streamProtocolTcp {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("can't receive tracks with different protocols"))
					return false
				}

				res := make(chan error)
				c.p.events <- programEventClientSetupPlay{res, c, path, streamProtocolTcp, 0, 0}
				err = <-res
				if err != nil {
					c.writeResError(req, gortsplib.StatusBadRequest, err)
					return false
				}

				interleaved := fmt.Sprintf("%d-%d", ((len(c.streamTracks) - 1) * 2), ((len(c.streamTracks)-1)*2)+1)

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

			// after ANNOUNCE, c.path is already set
			if path != c.path {
				c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path has changed"))
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
				if _, ok := c.p.conf.protocolsParsed[streamProtocolUdp]; !ok {
					c.writeResError(req, gortsplib.StatusUnsupportedTransport, fmt.Errorf("UDP streaming is disabled"))
					return false
				}

				rtpPort, rtcpPort := th.Ports("client_port")
				if rtpPort == 0 || rtcpPort == 0 {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not have valid client ports (%s)", req.Header["Transport"]))
					return false
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != streamProtocolUdp {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("can't publish tracks with different protocols"))
					return false
				}

				if len(c.streamTracks) >= len(c.streamSdpParsed.MediaDescriptions) {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("all the tracks have already been setup"))
					return false
				}

				res := make(chan error)
				c.p.events <- programEventClientSetupRecord{res, c, streamProtocolUdp, rtpPort, rtcpPort}
				err := <-res
				if err != nil {
					c.writeResError(req, gortsplib.StatusBadRequest, err)
					return false
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
				if _, ok := c.p.conf.protocolsParsed[streamProtocolTcp]; !ok {
					c.writeResError(req, gortsplib.StatusUnsupportedTransport, fmt.Errorf("TCP streaming is disabled"))
					return false
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != streamProtocolTcp {
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

				if len(c.streamTracks) >= len(c.streamSdpParsed.MediaDescriptions) {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("all the tracks have already been setup"))
					return false
				}

				res := make(chan error)
				c.p.events <- programEventClientSetupRecord{res, c, streamProtocolTcp, 0, 0}
				err := <-res
				if err != nil {
					c.writeResError(req, gortsplib.StatusBadRequest, err)
					return false
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

		if path != c.path {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path has changed"))
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

		if path != c.path {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path has changed"))
			return false
		}

		if len(c.streamTracks) != len(c.streamSdpParsed.MediaDescriptions) {
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

func (c *serverClient) runPlay(path string) {
	pconf := c.findConfForPath(path)

	if c.streamProtocol == streamProtocolTcp {
		c.writeBuf = newDoubleBuffer(2048)
		c.events = make(chan serverClientEvent)
	}

	done := make(chan struct{})
	c.p.events <- programEventClientPlay2{done, c}
	<-done

	c.log("is receiving on path '%s', %d %s via %s", c.path, len(c.streamTracks), func() string {
		if len(c.streamTracks) == 1 {
			return "track"
		}
		return "tracks"
	}(), c.streamProtocol)

	var runOnReadCmd *exec.Cmd
	if pconf.RunOnRead != "" {
		runOnReadCmd = exec.Command("/bin/sh", "-c", pconf.RunOnRead)
		runOnReadCmd.Stdout = os.Stdout
		runOnReadCmd.Stderr = os.Stderr
		err := runOnReadCmd.Start()
		if err != nil {
			c.log("ERR: %s", err)
		}
	}

	if c.streamProtocol == streamProtocolTcp {
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
				case serverClientEventFrameTcp:
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

	} else {
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
	}

	if runOnReadCmd != nil {
		runOnReadCmd.Process.Signal(os.Interrupt)
		runOnReadCmd.Wait()
	}
}

func (c *serverClient) runRecord(path string) {
	pconf := c.findConfForPath(path)

	c.RtcpReceivers = make([]*gortsplib.RtcpReceiver, len(c.streamTracks))
	for trackId := range c.streamTracks {
		c.RtcpReceivers[trackId] = gortsplib.NewRtcpReceiver()
	}

	done := make(chan struct{})
	c.p.events <- programEventClientRecord{done, c}
	<-done

	c.log("is publishing on path '%s', %d %s via %s", c.path, len(c.streamTracks), func() string {
		if len(c.streamTracks) == 1 {
			return "track"
		}
		return "tracks"
	}(), c.streamProtocol)

	var runOnPublishCmd *exec.Cmd
	if pconf.RunOnPublish != "" {
		runOnPublishCmd = exec.Command("/bin/sh", "-c", pconf.RunOnPublish)
		runOnPublishCmd.Stdout = os.Stdout
		runOnPublishCmd.Stderr = os.Stderr
		err := runOnPublishCmd.Start()
		if err != nil {
			c.log("ERR: %s", err)
		}
	}

	if c.streamProtocol == streamProtocolTcp {
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

					c.RtcpReceivers[frame.TrackId].OnFrame(frame.StreamType, frame.Content)
					c.p.events <- programEventClientFrameTcp{
						c.path,
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

		checkStreamTicker := time.NewTicker(clientCheckStreamInterval)
		receiverReportTicker := time.NewTicker(clientReceiverReportInterval)

	outer1:
		for {
			select {
			case err := <-readDone:
				if err != nil && err != io.EOF {
					c.log("ERR: %s", err)
				}
				break outer1

			case <-checkStreamTicker.C:
				for trackId := range c.streamTracks {
					if time.Since(c.RtcpReceivers[trackId].LastFrameTime()) >= c.p.conf.StreamDeadAfter {
						c.log("ERR: stream is dead")
						c.conn.NetConn().Close()
						<-readDone
						break outer1
					}
				}

			case <-receiverReportTicker.C:
				for trackId := range c.streamTracks {
					frame := c.RtcpReceivers[trackId].Report()
					c.conn.WriteFrame(&gortsplib.InterleavedFrame{
						TrackId:    trackId,
						StreamType: gortsplib.StreamTypeRtcp,
						Content:    frame,
					})
				}
			}
		}

		checkStreamTicker.Stop()
		receiverReportTicker.Stop()

	} else {
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
					if time.Since(c.RtcpReceivers[trackId].LastFrameTime()) >= c.p.conf.StreamDeadAfter {
						c.log("ERR: stream is dead")
						c.conn.NetConn().Close()
						<-readDone
						break outer2
					}
				}

			case <-receiverReportTicker.C:
				for trackId := range c.streamTracks {
					frame := c.RtcpReceivers[trackId].Report()
					c.p.rtcpl.writeChan <- &udpAddrBufPair{
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
	}

	done = make(chan struct{})
	c.p.events <- programEventClientRecordStop{done, c}
	<-done

	for trackId := range c.streamTracks {
		c.RtcpReceivers[trackId].Close()
	}

	if runOnPublishCmd != nil {
		runOnPublishCmd.Process.Signal(os.Interrupt)
		runOnPublishCmd.Wait()
	}
}
