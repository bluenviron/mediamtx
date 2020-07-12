package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/aler9/gortsplib"
	"gortc.io/sdp"
)

const (
	_CLIENT_CHECK_STREAM_INTERVAL    = 5 * time.Second
	_CLIENT_STREAM_DEAD_AFTER        = 15 * time.Second
	_CLIENT_RECEIVER_REPORT_INTERVAL = 10 * time.Second
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
	_CLIENT_STATE_STARTING serverClientState = iota
	_CLIENT_STATE_ANNOUNCE
	_CLIENT_STATE_PRE_PLAY
	_CLIENT_STATE_PLAY
	_CLIENT_STATE_PRE_RECORD
	_CLIENT_STATE_RECORD
)

func (cs serverClientState) String() string {
	switch cs {
	case _CLIENT_STATE_STARTING:
		return "STARTING"

	case _CLIENT_STATE_ANNOUNCE:
		return "ANNOUNCE"

	case _CLIENT_STATE_PRE_PLAY:
		return "PRE_PLAY"

	case _CLIENT_STATE_PLAY:
		return "PLAY"

	case _CLIENT_STATE_PRE_RECORD:
		return "PRE_RECORD"

	case _CLIENT_STATE_RECORD:
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
	streamSdpText   []byte       // only if publisher
	streamSdpParsed *sdp.Message // only if publisher
	streamProtocol  streamProtocol
	streamTracks    []*track
	rtcpReceivers   []*rtcpReceiver
	readBuf         *doubleBuffer
	writeBuf        *doubleBuffer

	events chan serverClientEvent
	done   chan struct{}
}

func newServerClient(p *program, nconn net.Conn) *serverClient {
	c := &serverClient{
		p: p,
		conn: gortsplib.NewConnServer(gortsplib.ConnServerConf{
			NConn:        nconn,
			ReadTimeout:  p.conf.ReadTimeout,
			WriteTimeout: p.conf.WriteTimeout,
		}),
		state:   _CLIENT_STATE_STARTING,
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
	return c.state == _CLIENT_STATE_RECORD
}

func (c *serverClient) publisherSdpText() []byte {
	return c.streamSdpText
}

func (c *serverClient) publisherSdpParsed() *sdp.Message {
	return c.streamSdpParsed
}

func (c *serverClient) run() {
	if c.p.conf.PreScript != "" {
		preScript := exec.Command(c.p.conf.PreScript)
		err := preScript.Run()
		if err != nil {
			c.log("ERR: %s", err)
		}
	}

outer:
	for {
		switch c.state {
		case _CLIENT_STATE_PLAY:
			ok := c.runPlay()
			if !ok {
				break outer
			}

		case _CLIENT_STATE_RECORD:
			ok := c.runRecord()
			if !ok {
				break outer
			}

		default:
			ok := c.runNormal()
			if !ok {
				break outer
			}
		}
	}

	c.conn.NetConn().Close() // close socket in case it has not been closed yet

	func() {
		if c.p.conf.PostScript != "" {
			postScript := exec.Command(c.p.conf.PostScript)
			err := postScript.Run()
			if err != nil {
				c.log("ERR: %s", err)
			}
		}
	}()

	close(c.done) // close() never blocks
}

var errClientChangeRunMode = errors.New("change run mode")
var errClientTerminate = errors.New("terminate")

func (c *serverClient) runNormal() bool {
	var ret bool

outer:
	for {
		req, err := c.conn.ReadRequest()
		if err != nil {
			if err != io.EOF {
				c.log("ERR: %s", err)
			}
			ret = false
			break outer
		}

		err = c.handleRequest(req)
		switch err {
		case errClientChangeRunMode:
			ret = true
			break outer

		case errClientTerminate:
			ret = false
			break outer
		}
	}

	if !ret {
		done := make(chan struct{})
		c.p.events <- programEventClientClose{done, c}
		<-done
	}

	return ret
}

func (c *serverClient) runPlay() bool {
	if c.streamProtocol == _STREAM_PROTOCOL_TCP {
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
					c.conn.WriteInterleavedFrame(evt.frame)
				}
			}
		}

		go func() {
			for range c.events {
			}
		}()

		done := make(chan struct{})
		c.p.events <- programEventClientClose{done, c}
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

			err = c.handleRequest(req)
			if err != nil {
				break
			}
		}

		done := make(chan struct{})
		c.p.events <- programEventClientClose{done, c}
		<-done
	}

	return false
}

func (c *serverClient) runRecord() bool {
	if c.streamProtocol == _STREAM_PROTOCOL_TCP {
		frame := &gortsplib.InterleavedFrame{}

		readDone := make(chan error)
		go func() {
			for {
				frame.Content = c.readBuf.swap()
				frame.Content = frame.Content[:cap(frame.Content)]
				recv, err := c.conn.ReadInterleavedFrameOrRequest(frame)
				if err != nil {
					readDone <- err
					break
				}

				switch recvt := recv.(type) {
				case *gortsplib.InterleavedFrame:
					trackId, trackFlowType := interleavedChannelToTrackFlowType(frame.Channel)
					if trackId >= len(c.streamTracks) {
						c.log("ERR: invalid track id '%d'", trackId)
						readDone <- nil
						break
					}

					c.rtcpReceivers[trackId].onFrame(trackFlowType, frame.Content)
					c.p.events <- programEventClientFrameTcp{
						c.path,
						trackId,
						trackFlowType,
						frame.Content,
					}

				case *gortsplib.Request:
					err := c.handleRequest(recvt)
					if err != nil {
						readDone <- nil
						break
					}
				}
			}
		}()

		checkStreamTicker := time.NewTicker(_CLIENT_CHECK_STREAM_INTERVAL)
		receiverReportTicker := time.NewTicker(_CLIENT_RECEIVER_REPORT_INTERVAL)

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
					if time.Since(c.rtcpReceivers[trackId].lastFrameTime()) >= _CLIENT_STREAM_DEAD_AFTER {
						c.log("ERR: stream is dead")
						c.conn.NetConn().Close()
						<-readDone
						break outer1
					}
				}

			case <-receiverReportTicker.C:
				for trackId := range c.streamTracks {
					channel := trackFlowTypeToInterleavedChannel(trackId, _TRACK_FLOW_TYPE_RTCP)

					frame := c.rtcpReceivers[trackId].report()
					c.conn.WriteInterleavedFrame(&gortsplib.InterleavedFrame{
						Channel: channel,
						Content: frame,
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

				err = c.handleRequest(req)
				if err != nil {
					readDone <- nil // err is not needed
					break
				}
			}
		}()

		checkStreamTicker := time.NewTicker(_CLIENT_CHECK_STREAM_INTERVAL)
		receiverReportTicker := time.NewTicker(_CLIENT_RECEIVER_REPORT_INTERVAL)

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
					if time.Since(c.rtcpReceivers[trackId].lastFrameTime()) >= _CLIENT_STREAM_DEAD_AFTER {
						c.log("ERR: stream is dead")
						c.conn.NetConn().Close()
						<-readDone
						break outer2
					}
				}

			case <-receiverReportTicker.C:
				for trackId := range c.streamTracks {
					frame := c.rtcpReceivers[trackId].report()
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

	done := make(chan struct{})
	c.p.events <- programEventClientClose{done, c}
	<-done

	for trackId := range c.streamTracks {
		c.rtcpReceivers[trackId].close()
	}

	return false
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

func (c *serverClient) handleRequest(req *gortsplib.Request) error {
	c.log(string(req.Method))

	cseq, ok := req.Header["CSeq"]
	if !ok || len(cseq) != 1 {
		c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("cseq missing"))
		return errClientTerminate
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
				"Public": []string{strings.Join([]string{
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
		if c.state != _CLIENT_STATE_STARTING {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, _CLIENT_STATE_STARTING))
			return errClientTerminate
		}

		pconf := c.findConfForPath(path)
		if pconf == nil {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("unable to find a valid configuration for path '%s'", path))
			return errClientTerminate
		}

		err := c.authenticate(pconf.readIpsParsed, pconf.ReadUser, pconf.ReadPass, req)
		if err != nil {
			if err == errAuthCritical {
				return errClientTerminate
			}
			return nil
		}

		res := make(chan []byte)
		c.p.events <- programEventClientDescribe{path, res}
		sdp := <-res
		if sdp == nil {
			c.writeResError(req, gortsplib.StatusNotFound, fmt.Errorf("no one is publishing on path '%s'", path))
			return errClientTerminate
		}

		c.conn.WriteResponse(&gortsplib.Response{
			StatusCode: gortsplib.StatusOK,
			Header: gortsplib.Header{
				"CSeq":         cseq,
				"Content-Base": []string{req.Url.String() + "/"},
				"Content-Type": []string{"application/sdp"},
			},
			Content: sdp,
		})
		return nil

	case gortsplib.ANNOUNCE:
		if c.state != _CLIENT_STATE_STARTING {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, _CLIENT_STATE_STARTING))
			return errClientTerminate
		}

		pconf := c.findConfForPath(path)
		if pconf == nil {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("unable to find a valid configuration for path '%s'", path))
			return errClientTerminate
		}

		err := c.authenticate(pconf.publishIpsParsed, pconf.PublishUser, pconf.PublishPass, req)
		if err != nil {
			if err == errAuthCritical {
				return errClientTerminate
			}
			return nil
		}

		ct, ok := req.Header["Content-Type"]
		if !ok || len(ct) != 1 {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("Content-Type header missing"))
			return errClientTerminate
		}

		if ct[0] != "application/sdp" {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("unsupported Content-Type '%s'", ct))
			return errClientTerminate
		}

		sdpParsed, err := gortsplib.SDPParse(req.Content)
		if err != nil {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("invalid SDP: %s", err))
			return errClientTerminate
		}
		sdpParsed, req.Content = gortsplib.SDPFilter(sdpParsed, req.Content)

		if len(path) == 0 {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path can't be empty"))
			return errClientTerminate
		}

		res := make(chan error)
		c.p.events <- programEventClientAnnounce{res, c, path}
		err = <-res
		if err != nil {
			c.writeResError(req, gortsplib.StatusBadRequest, err)
			return errClientTerminate
		}

		c.streamSdpText = req.Content
		c.streamSdpParsed = sdpParsed

		c.conn.WriteResponse(&gortsplib.Response{
			StatusCode: gortsplib.StatusOK,
			Header: gortsplib.Header{
				"CSeq": cseq,
			},
		})
		return nil

	case gortsplib.SETUP:
		tsRaw, ok := req.Header["Transport"]
		if !ok || len(tsRaw) != 1 {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header missing"))
			return errClientTerminate
		}

		th := gortsplib.ReadHeaderTransport(tsRaw[0])
		if _, ok := th["multicast"]; ok {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("multicast is not supported"))
			return errClientTerminate
		}

		switch c.state {
		// play
		case _CLIENT_STATE_STARTING, _CLIENT_STATE_PRE_PLAY:
			pconf := c.findConfForPath(path)
			if pconf == nil {
				c.writeResError(req, gortsplib.StatusBadRequest,
					fmt.Errorf("unable to find a valid configuration for path '%s'", path))
				return errClientTerminate
			}

			err := c.authenticate(pconf.readIpsParsed, pconf.ReadUser, pconf.ReadPass, req)
			if err != nil {
				if err == errAuthCritical {
					return errClientTerminate
				}
				return nil
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
				if _, ok := c.p.conf.protocolsParsed[_STREAM_PROTOCOL_UDP]; !ok {
					c.writeResError(req, gortsplib.StatusUnsupportedTransport, fmt.Errorf("UDP streaming is disabled"))
					return errClientTerminate
				}

				rtpPort, rtcpPort := th.GetPorts("client_port")
				if rtpPort == 0 || rtcpPort == 0 {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not have valid client ports (%s)", tsRaw[0]))
					return errClientTerminate
				}

				if c.path != "" && path != c.path {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path has changed"))
					return errClientTerminate
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != _STREAM_PROTOCOL_UDP {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("can't receive tracks with different protocols"))
					return errClientTerminate
				}

				res := make(chan error)
				c.p.events <- programEventClientSetupPlay{res, c, path, _STREAM_PROTOCOL_UDP, rtpPort, rtcpPort}
				err = <-res
				if err != nil {
					c.writeResError(req, gortsplib.StatusBadRequest, err)
					return errClientTerminate
				}

				c.conn.WriteResponse(&gortsplib.Response{
					StatusCode: gortsplib.StatusOK,
					Header: gortsplib.Header{
						"CSeq": cseq,
						"Transport": []string{strings.Join([]string{
							"RTP/AVP/UDP",
							"unicast",
							fmt.Sprintf("client_port=%d-%d", rtpPort, rtcpPort),
							fmt.Sprintf("server_port=%d-%d", c.p.conf.RtpPort, c.p.conf.RtcpPort),
						}, ";")},
						"Session": []string{"12345678"},
					},
				})
				return nil

				// play via TCP
			} else if _, ok := th["RTP/AVP/TCP"]; ok {
				if _, ok := c.p.conf.protocolsParsed[_STREAM_PROTOCOL_TCP]; !ok {
					c.writeResError(req, gortsplib.StatusUnsupportedTransport, fmt.Errorf("TCP streaming is disabled"))
					return errClientTerminate
				}

				if c.path != "" && path != c.path {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path has changed"))
					return errClientTerminate
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != _STREAM_PROTOCOL_TCP {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("can't receive tracks with different protocols"))
					return errClientTerminate
				}

				res := make(chan error)
				c.p.events <- programEventClientSetupPlay{res, c, path, _STREAM_PROTOCOL_TCP, 0, 0}
				err = <-res
				if err != nil {
					c.writeResError(req, gortsplib.StatusBadRequest, err)
					return errClientTerminate
				}

				interleaved := fmt.Sprintf("%d-%d", ((len(c.streamTracks) - 1) * 2), ((len(c.streamTracks)-1)*2)+1)

				c.conn.WriteResponse(&gortsplib.Response{
					StatusCode: gortsplib.StatusOK,
					Header: gortsplib.Header{
						"CSeq": cseq,
						"Transport": []string{strings.Join([]string{
							"RTP/AVP/TCP",
							"unicast",
							fmt.Sprintf("interleaved=%s", interleaved),
						}, ";")},
						"Session": []string{"12345678"},
					},
				})
				return nil

			} else {
				c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not contain a valid protocol (RTP/AVP, RTP/AVP/UDP or RTP/AVP/TCP) (%s)", tsRaw[0]))
				return errClientTerminate
			}

		// record
		case _CLIENT_STATE_ANNOUNCE, _CLIENT_STATE_PRE_RECORD:
			if _, ok := th["mode=record"]; !ok {
				c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not contain mode=record"))
				return errClientTerminate
			}

			// after ANNOUNCE, c.path is already set
			if path != c.path {
				c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path has changed"))
				return errClientTerminate
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
				if _, ok := c.p.conf.protocolsParsed[_STREAM_PROTOCOL_UDP]; !ok {
					c.writeResError(req, gortsplib.StatusUnsupportedTransport, fmt.Errorf("UDP streaming is disabled"))
					return errClientTerminate
				}

				rtpPort, rtcpPort := th.GetPorts("client_port")
				if rtpPort == 0 || rtcpPort == 0 {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not have valid client ports (%s)", tsRaw[0]))
					return errClientTerminate
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != _STREAM_PROTOCOL_UDP {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("can't publish tracks with different protocols"))
					return errClientTerminate
				}

				if len(c.streamTracks) >= len(c.streamSdpParsed.Medias) {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("all the tracks have already been setup"))
					return errClientTerminate
				}

				res := make(chan error)
				c.p.events <- programEventClientSetupRecord{res, c, _STREAM_PROTOCOL_UDP, rtpPort, rtcpPort}
				err := <-res
				if err != nil {
					c.writeResError(req, gortsplib.StatusBadRequest, err)
					return errClientTerminate
				}

				c.conn.WriteResponse(&gortsplib.Response{
					StatusCode: gortsplib.StatusOK,
					Header: gortsplib.Header{
						"CSeq": cseq,
						"Transport": []string{strings.Join([]string{
							"RTP/AVP/UDP",
							"unicast",
							fmt.Sprintf("client_port=%d-%d", rtpPort, rtcpPort),
							fmt.Sprintf("server_port=%d-%d", c.p.conf.RtpPort, c.p.conf.RtcpPort),
						}, ";")},
						"Session": []string{"12345678"},
					},
				})
				return nil

				// record via TCP
			} else if _, ok := th["RTP/AVP/TCP"]; ok {
				if _, ok := c.p.conf.protocolsParsed[_STREAM_PROTOCOL_TCP]; !ok {
					c.writeResError(req, gortsplib.StatusUnsupportedTransport, fmt.Errorf("TCP streaming is disabled"))
					return errClientTerminate
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != _STREAM_PROTOCOL_TCP {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("can't publish tracks with different protocols"))
					return errClientTerminate
				}

				interleaved := th.GetValue("interleaved")
				if interleaved == "" {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not contain the interleaved field"))
					return errClientTerminate
				}

				expInterleaved := fmt.Sprintf("%d-%d", 0+len(c.streamTracks)*2, 1+len(c.streamTracks)*2)
				if interleaved != expInterleaved {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("wrong interleaved value, expected '%s', got '%s'", expInterleaved, interleaved))
					return errClientTerminate
				}

				if len(c.streamTracks) >= len(c.streamSdpParsed.Medias) {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("all the tracks have already been setup"))
					return errClientTerminate
				}

				res := make(chan error)
				c.p.events <- programEventClientSetupRecord{res, c, _STREAM_PROTOCOL_TCP, 0, 0}
				err := <-res
				if err != nil {
					c.writeResError(req, gortsplib.StatusBadRequest, err)
					return errClientTerminate
				}

				c.conn.WriteResponse(&gortsplib.Response{
					StatusCode: gortsplib.StatusOK,
					Header: gortsplib.Header{
						"CSeq": cseq,
						"Transport": []string{strings.Join([]string{
							"RTP/AVP/TCP",
							"unicast",
							fmt.Sprintf("interleaved=%s", interleaved),
						}, ";")},
						"Session": []string{"12345678"},
					},
				})
				return nil

			} else {
				c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not contain a valid protocol (RTP/AVP, RTP/AVP/UDP or RTP/AVP/TCP) (%s)", tsRaw[0]))
				return errClientTerminate
			}

		default:
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("client is in state '%s'", c.state))
			return errClientTerminate
		}

	case gortsplib.PLAY:
		if c.state != _CLIENT_STATE_PRE_PLAY {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, _CLIENT_STATE_PRE_PLAY))
			return errClientTerminate
		}

		if path != c.path {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path has changed"))
			return errClientTerminate
		}

		// check publisher existence
		res := make(chan error)
		c.p.events <- programEventClientPlay1{res, c}
		err := <-res
		if err != nil {
			c.writeResError(req, gortsplib.StatusBadRequest, err)
			return errClientTerminate
		}

		// write response before setting state
		// otherwise, in case of TCP connections, RTP packets could be sent
		// before the response
		c.conn.WriteResponse(&gortsplib.Response{
			StatusCode: gortsplib.StatusOK,
			Header: gortsplib.Header{
				"CSeq":    cseq,
				"Session": []string{"12345678"},
			},
		})

		if c.streamProtocol == _STREAM_PROTOCOL_TCP {
			c.writeBuf = newDoubleBuffer(2048)
			c.events = make(chan serverClientEvent)
		}

		// set state
		res = make(chan error)
		c.p.events <- programEventClientPlay2{res, c}
		<-res

		c.log("is receiving on path '%s', %d %s via %s", c.path, len(c.streamTracks), func() string {
			if len(c.streamTracks) == 1 {
				return "track"
			}
			return "tracks"
		}(), c.streamProtocol)

		return errClientChangeRunMode

	case gortsplib.RECORD:
		if c.state != _CLIENT_STATE_PRE_RECORD {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, _CLIENT_STATE_PRE_RECORD))
			return errClientTerminate
		}

		if path != c.path {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path has changed"))
			return errClientTerminate
		}

		if len(c.streamTracks) != len(c.streamSdpParsed.Medias) {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("not all tracks have been setup"))
			return errClientTerminate
		}

		c.conn.WriteResponse(&gortsplib.Response{
			StatusCode: gortsplib.StatusOK,
			Header: gortsplib.Header{
				"CSeq":    cseq,
				"Session": []string{"12345678"},
			},
		})

		c.rtcpReceivers = make([]*rtcpReceiver, len(c.streamTracks))
		for trackId := range c.streamTracks {
			c.rtcpReceivers[trackId] = newRtcpReceiver()
		}

		res := make(chan error)
		c.p.events <- programEventClientRecord{res, c}
		<-res

		c.log("is publishing on path '%s', %d %s via %s", c.path, len(c.streamTracks), func() string {
			if len(c.streamTracks) == 1 {
				return "track"
			}
			return "tracks"
		}(), c.streamProtocol)

		return errClientChangeRunMode

	case gortsplib.TEARDOWN:
		// close connection silently
		return errClientTerminate

	default:
		c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("unhandled method '%s'", req.Method))
		return errClientTerminate
	}
}
