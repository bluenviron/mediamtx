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
	_UDP_CHECK_STREAM_INTERVAL = 5 * time.Second
	_UDP_STREAM_DEAD_AFTER     = 10 * time.Second
)

func interleavedChannelToTrack(channel uint8) (int, trackFlowType) {
	if (channel % 2) == 0 {
		return int(channel / 2), _TRACK_FLOW_RTP
	}
	return int((channel - 1) / 2), _TRACK_FLOW_RTCP
}

func trackToInterleavedChannel(id int, trackFlowType trackFlowType) uint8 {
	if trackFlowType == _TRACK_FLOW_RTP {
		return uint8(id * 2)
	}
	return uint8((id * 2) + 1)
}

type clientState int

const (
	_CLIENT_STATE_STARTING clientState = iota
	_CLIENT_STATE_ANNOUNCE
	_CLIENT_STATE_PRE_PLAY
	_CLIENT_STATE_PLAY
	_CLIENT_STATE_PRE_RECORD
	_CLIENT_STATE_RECORD
)

func (cs clientState) String() string {
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
	p                    *program
	conn                 *gortsplib.ConnServer
	state                clientState
	path                 string
	publishAuth          *gortsplib.AuthServer
	readAuth             *gortsplib.AuthServer
	streamSdpText        []byte       // filled only if publisher
	streamSdpParsed      *sdp.Message // filled only if publisher
	streamProtocol       streamProtocol
	streamTracks         []*track
	udpLastFrameTime     time.Time
	udpCheckStreamTicker *time.Ticker
	readBuf1             []byte
	readBuf2             []byte
	readCurBuf           bool
	writeBuf1            []byte
	writeBuf2            []byte
	writeCurBuf          bool

	writec chan *gortsplib.InterleavedFrame
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
		state:     _CLIENT_STATE_STARTING,
		readBuf1:  make([]byte, 0, 512*1024),
		readBuf2:  make([]byte, 0, 512*1024),
		writeBuf1: make([]byte, 2048),
		writeBuf2: make([]byte, 2048),
		writec:    make(chan *gortsplib.InterleavedFrame),
		done:      make(chan struct{}),
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

func (c *serverClient) run() {
	if c.p.conf.PreScript != "" {
		preScript := exec.Command(c.p.conf.PreScript)
		err := preScript.Run()
		if err != nil {
			c.log("ERR: %s", err)
		}
	}

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

	if c.udpCheckStreamTicker != nil {
		c.udpCheckStreamTicker.Stop()
	}

	go func() {
		for range c.writec {
		}
	}()

	func() {
		if c.p.conf.PostScript != "" {
			postScript := exec.Command(c.p.conf.PostScript)
			err := postScript.Run()
			if err != nil {
				c.log("ERR: %s", err)
			}
		}
	}()

	done := make(chan struct{})
	c.p.events <- programEventClientClose{done, c}
	<-done

	close(c.writec)

	close(c.done)
}

func (c *serverClient) close() {
	c.conn.NetConn().Close()
	<-c.done
}

func (c *serverClient) writeFrame(channel uint8, inbuf []byte) {
	var buf []byte
	if !c.writeCurBuf {
		buf = c.writeBuf1
	} else {
		buf = c.writeBuf2
	}

	buf = buf[:len(inbuf)]
	copy(buf, inbuf)
	c.writeCurBuf = !c.writeCurBuf

	c.writec <- &gortsplib.InterleavedFrame{
		Channel: channel,
		Content: buf,
	}
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

var errAuthCritical = errors.New("auth critical")
var errAuthNotCritical = errors.New("auth not critical")

func (c *serverClient) validateAuth(req *gortsplib.Request, user string, pass string, auth **gortsplib.AuthServer, ips []interface{}) error {
	err := func() error {
		if ips == nil {
			return nil
		}

		connIp := c.conn.NetConn().LocalAddr().(*net.TCPAddr).IP

		for _, item := range ips {
			switch titem := item.(type) {
			case net.IP:
				if titem.Equal(connIp) {
					return nil
				}

			case *net.IPNet:
				if titem.Contains(connIp) {
					return nil
				}
			}
		}

		c.log("ERR: ip '%s' not allowed", connIp)
		return errAuthCritical
	}()
	if err != nil {
		return err
	}

	err = func() error {
		if user == "" {
			return nil
		}

		initialRequest := false
		if *auth == nil {
			initialRequest = true
			*auth = gortsplib.NewAuthServer(user, pass, nil)
		}

		err := (*auth).ValidateHeader(req.Header["Authorization"], req.Method, req.Url)
		if err != nil {
			if !initialRequest {
				c.log("ERR: unauthorized: %s", err)
			}

			c.conn.WriteResponse(&gortsplib.Response{
				StatusCode: gortsplib.StatusUnauthorized,
				Header: gortsplib.Header{
					"CSeq":             []string{req.Header["CSeq"][0]},
					"WWW-Authenticate": (*auth).GenerateHeader(),
				},
			})

			if !initialRequest {
				return errAuthCritical
			}

			return errAuthNotCritical
		}

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
				"Public": []string{strings.Join([]string{
					string(gortsplib.DESCRIBE),
					string(gortsplib.ANNOUNCE),
					string(gortsplib.SETUP),
					string(gortsplib.PLAY),
					string(gortsplib.PAUSE),
					string(gortsplib.RECORD),
					string(gortsplib.TEARDOWN),
				}, ", ")},
			},
		})
		return true

	case gortsplib.DESCRIBE:
		if c.state != _CLIENT_STATE_STARTING {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, _CLIENT_STATE_STARTING))
			return false
		}

		err := c.validateAuth(req, c.p.conf.ReadUser, c.p.conf.ReadPass, &c.readAuth, c.p.readIps)
		if err != nil {
			if err == errAuthCritical {
				return false
			}
			return true
		}

		res := make(chan []byte)
		c.p.events <- programEventClientGetStreamSdp{path, res}
		sdp := <-res
		if sdp == nil {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("no one is streaming on path '%s'", path))
			return false
		}

		c.conn.WriteResponse(&gortsplib.Response{
			StatusCode: gortsplib.StatusOK,
			Header: gortsplib.Header{
				"CSeq":         cseq,
				"Content-Base": []string{req.Url.String()},
				"Content-Type": []string{"application/sdp"},
			},
			Content: sdp,
		})
		return true

	case gortsplib.ANNOUNCE:
		if c.state != _CLIENT_STATE_STARTING {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, _CLIENT_STATE_STARTING))
			return false
		}

		err := c.validateAuth(req, c.p.conf.PublishUser, c.p.conf.PublishPass, &c.publishAuth, c.p.publishIps)
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

		sdpParsed, err := gortsplib.SDPParse(req.Content)
		if err != nil {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("invalid SDP: %s", err))
			return false
		}
		sdpParsed, req.Content = gortsplib.SDPFilter(sdpParsed, req.Content)

		if len(path) == 0 {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path can't be empty"))
			return false
		}

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
		tsRaw, ok := req.Header["Transport"]
		if !ok || len(tsRaw) != 1 {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header missing"))
			return false
		}

		th := gortsplib.ReadHeaderTransport(tsRaw[0])
		if _, ok := th["multicast"]; ok {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("multicast is not supported"))
			return false
		}

		switch c.state {
		// play
		case _CLIENT_STATE_STARTING, _CLIENT_STATE_PRE_PLAY:
			err := c.validateAuth(req, c.p.conf.ReadUser, c.p.conf.ReadPass, &c.readAuth, c.p.readIps)
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
				if _, ok := c.p.protocols[_STREAM_PROTOCOL_UDP]; !ok {
					c.writeResError(req, gortsplib.StatusUnsupportedTransport, fmt.Errorf("UDP streaming is disabled"))
					return false
				}

				rtpPort, rtcpPort := th.GetPorts("client_port")
				if rtpPort == 0 || rtcpPort == 0 {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not have valid client ports (%s)", tsRaw[0]))
					return false
				}

				if c.path != "" && path != c.path {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path has changed"))
					return false
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != _STREAM_PROTOCOL_UDP {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("can't receive tracks with different protocols"))
					return false
				}

				res := make(chan error)
				c.p.events <- programEventClientSetupPlay{res, c, path, _STREAM_PROTOCOL_UDP, rtpPort, rtcpPort}
				err = <-res
				if err != nil {
					c.writeResError(req, gortsplib.StatusBadRequest, err)
					return false
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
				return true

				// play via TCP
			} else if _, ok := th["RTP/AVP/TCP"]; ok {
				if _, ok := c.p.protocols[_STREAM_PROTOCOL_TCP]; !ok {
					c.writeResError(req, gortsplib.StatusUnsupportedTransport, fmt.Errorf("TCP streaming is disabled"))
					return false
				}

				if c.path != "" && path != c.path {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path has changed"))
					return false
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != _STREAM_PROTOCOL_TCP {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("can't receive tracks with different protocols"))
					return false
				}

				res := make(chan error)
				c.p.events <- programEventClientSetupPlay{res, c, path, _STREAM_PROTOCOL_TCP, 0, 0}
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
						"Transport": []string{strings.Join([]string{
							"RTP/AVP/TCP",
							"unicast",
							fmt.Sprintf("interleaved=%s", interleaved),
						}, ";")},
						"Session": []string{"12345678"},
					},
				})
				return true

			} else {
				c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not contain a valid protocol (RTP/AVP, RTP/AVP/UDP or RTP/AVP/TCP) (%s)", tsRaw[0]))
				return false
			}

		// record
		case _CLIENT_STATE_ANNOUNCE, _CLIENT_STATE_PRE_RECORD:
			if _, ok := th["mode=record"]; !ok {
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
				if _, ok := c.p.protocols[_STREAM_PROTOCOL_UDP]; !ok {
					c.writeResError(req, gortsplib.StatusUnsupportedTransport, fmt.Errorf("UDP streaming is disabled"))
					return false
				}

				rtpPort, rtcpPort := th.GetPorts("client_port")
				if rtpPort == 0 || rtcpPort == 0 {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not have valid client ports (%s)", tsRaw[0]))
					return false
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != _STREAM_PROTOCOL_UDP {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("can't publish tracks with different protocols"))
					return false
				}

				if len(c.streamTracks) >= len(c.streamSdpParsed.Medias) {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("all the tracks have already been setup"))
					return false
				}

				res := make(chan error)
				c.p.events <- programEventClientSetupRecord{res, c, _STREAM_PROTOCOL_UDP, rtpPort, rtcpPort}
				err := <-res
				if err != nil {
					c.writeResError(req, gortsplib.StatusBadRequest, err)
					return false
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
				return true

				// record via TCP
			} else if _, ok := th["RTP/AVP/TCP"]; ok {
				if _, ok := c.p.protocols[_STREAM_PROTOCOL_TCP]; !ok {
					c.writeResError(req, gortsplib.StatusUnsupportedTransport, fmt.Errorf("TCP streaming is disabled"))
					return false
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != _STREAM_PROTOCOL_TCP {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("can't publish tracks with different protocols"))
					return false
				}

				interleaved := th.GetValue("interleaved")
				if interleaved == "" {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not contain the interleaved field"))
					return false
				}

				expInterleaved := fmt.Sprintf("%d-%d", 0+len(c.streamTracks)*2, 1+len(c.streamTracks)*2)
				if interleaved != expInterleaved {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("wrong interleaved value, expected '%s', got '%s'", expInterleaved, interleaved))
					return false
				}

				if len(c.streamTracks) >= len(c.streamSdpParsed.Medias) {
					c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("all the tracks have already been setup"))
					return false
				}

				res := make(chan error)
				c.p.events <- programEventClientSetupRecord{res, c, _STREAM_PROTOCOL_TCP, 0, 0}
				err := <-res
				if err != nil {
					c.writeResError(req, gortsplib.StatusBadRequest, err)
					return false
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
				return true

			} else {
				c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not contain a valid protocol (RTP/AVP, RTP/AVP/UDP or RTP/AVP/TCP) (%s)", tsRaw[0]))
				return false
			}

		default:
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("client is in state '%s'", c.state))
			return false
		}

	case gortsplib.PLAY:
		if c.state != _CLIENT_STATE_PRE_PLAY {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, _CLIENT_STATE_PRE_PLAY))
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
				"Session": []string{"12345678"},
			},
		})

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

		// when protocol is TCP, the RTSP connection becomes a RTP connection
		if c.streamProtocol == _STREAM_PROTOCOL_TCP {
			// write RTP frames sequentially
			go func() {
				for frame := range c.writec {
					c.conn.WriteInterleavedFrame(frame)
				}
			}()

			// receive RTP feedback, do not parse it, wait until connection closes
			buf := make([]byte, 2048)
			for {
				_, err := c.conn.NetConn().Read(buf)
				if err != nil {
					if err != io.EOF {
						c.log("ERR: %s", err)
					}
					return false
				}
			}
		}

		return true

	case gortsplib.PAUSE:
		if c.state != _CLIENT_STATE_PLAY {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, _CLIENT_STATE_PLAY))
			return false
		}

		if path != c.path {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path has changed"))
			return false
		}

		c.log("paused")

		res := make(chan error)
		c.p.events <- programEventClientPause{res, c}
		<-res

		c.conn.WriteResponse(&gortsplib.Response{
			StatusCode: gortsplib.StatusOK,
			Header: gortsplib.Header{
				"CSeq":    cseq,
				"Session": []string{"12345678"},
			},
		})
		return true

	case gortsplib.RECORD:
		if c.state != _CLIENT_STATE_PRE_RECORD {
			c.writeResError(req, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, _CLIENT_STATE_PRE_RECORD))
			return false
		}

		if path != c.path {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("path has changed"))
			return false
		}

		if len(c.streamTracks) != len(c.streamSdpParsed.Medias) {
			c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("not all tracks have been setup"))
			return false
		}

		c.conn.WriteResponse(&gortsplib.Response{
			StatusCode: gortsplib.StatusOK,
			Header: gortsplib.Header{
				"CSeq":    cseq,
				"Session": []string{"12345678"},
			},
		})

		res := make(chan error)
		c.p.events <- programEventClientRecord{res, c}
		<-res

		c.log("is publishing on path '%s', %d %s via %s", c.path, len(c.streamTracks), func() string {
			if len(c.streamTracks) == 1 {
				return "track"
			}
			return "tracks"
		}(), c.streamProtocol)

		// when protocol is TCP, the RTSP connection becomes a RTP connection
		// receive RTP data and parse it
		if c.streamProtocol == _STREAM_PROTOCOL_TCP {
			frame := &gortsplib.InterleavedFrame{}
			for {
				if !c.readCurBuf {
					frame.Content = c.readBuf1
				} else {
					frame.Content = c.readBuf2
				}

				frame.Content = frame.Content[:cap(frame.Content)]
				c.readCurBuf = !c.readCurBuf

				err := c.conn.ReadInterleavedFrame(frame)
				if err != nil {
					if err != io.EOF {
						c.log("ERR: %s", err)
					}
					return false
				}

				trackId, trackFlowType := interleavedChannelToTrack(frame.Channel)

				if trackId >= len(c.streamTracks) {
					c.log("ERR: invalid track id '%d'", trackId)
					return false
				}

				c.p.events <- programEventFrameTcp{
					c.path,
					trackId,
					trackFlowType,
					frame.Content,
				}
			}
		} else {
			c.udpLastFrameTime = time.Now()
			c.udpCheckStreamTicker = time.NewTicker(_UDP_CHECK_STREAM_INTERVAL)

			go func() {
				for range c.udpCheckStreamTicker.C {
					if time.Since(c.udpLastFrameTime) >= _UDP_STREAM_DEAD_AFTER {
						c.log("ERR: stream is dead")
						c.conn.NetConn().Close()
						break
					}
				}
			}()
		}

		return true

	case gortsplib.TEARDOWN:
		// close connection silently
		return false

	default:
		c.writeResError(req, gortsplib.StatusBadRequest, fmt.Errorf("unhandled method '%s'", req.Method))
		return false
	}
}
