package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/aler9/gortsplib"
	"gortc.io/sdp"
)

func interleavedChannelToTrack(channel int) (int, trackFlow) {
	if (channel % 2) == 0 {
		return (channel / 2), _TRACK_FLOW_RTP
	}
	return ((channel - 1) / 2), _TRACK_FLOW_RTCP
}

func trackToInterleavedChannel(id int, flow trackFlow) int {
	if flow == _TRACK_FLOW_RTP {
		return id * 2
	}
	return (id * 2) + 1
}

type transportHeader map[string]struct{}

func newTransportHeader(in string) transportHeader {
	th := make(map[string]struct{})
	for _, t := range strings.Split(in, ";") {
		th[t] = struct{}{}
	}
	return th
}

func (th transportHeader) getKeyValue(key string) string {
	prefix := key + "="
	for t := range th {
		if strings.HasPrefix(t, prefix) {
			return t[len(prefix):]
		}
	}
	return ""
}

func (th transportHeader) getClientPorts() (int, int) {
	val := th.getKeyValue("client_port")
	if val == "" {
		return 0, 0
	}

	ports := strings.Split(val, "-")
	if len(ports) != 2 {
		return 0, 0
	}

	port1, err := strconv.ParseInt(ports[0], 10, 64)
	if err != nil {
		return 0, 0
	}

	port2, err := strconv.ParseInt(ports[1], 10, 64)
	if err != nil {
		return 0, 0
	}

	return int(port1), int(port2)
}

type client struct {
	p               *program
	rconn           *gortsplib.Conn
	state           string
	ip              net.IP
	path            string
	streamSdpText   []byte       // filled only if publisher
	streamSdpParsed *sdp.Message // filled only if publisher
	streamProtocol  streamProtocol
	streamTracks    []*track
}

func newClient(p *program, nconn net.Conn) *client {
	c := &client{
		p:     p,
		rconn: gortsplib.NewConn(nconn),
		state: "STARTING",
	}

	c.p.mutex.Lock()
	c.p.clients[c] = struct{}{}
	c.p.mutex.Unlock()

	return c
}

func (c *client) close() error {
	// already deleted
	if _, ok := c.p.clients[c]; !ok {
		return nil
	}

	delete(c.p.clients, c)
	c.rconn.Close()

	if c.path != "" {
		if pub, ok := c.p.publishers[c.path]; ok && pub == c {
			delete(c.p.publishers, c.path)

			// if the publisher has disconnected
			// close all other connections that share the same path
			for oc := range c.p.clients {
				if oc.path == c.path {
					oc.close()
				}
			}
		}
	}
	return nil
}

func (c *client) log(format string, args ...interface{}) {
	format = "[RTSP client " + c.rconn.RemoteAddr().String() + "] " + format
	log.Printf(format, args...)
}

func (c *client) run() {
	defer c.log("disconnected")
	defer func() {
		c.p.mutex.Lock()
		defer c.p.mutex.Unlock()
		c.close()
	}()

	ipstr, _, _ := net.SplitHostPort(c.rconn.RemoteAddr().String())
	c.ip = net.ParseIP(ipstr)

	c.log("connected")

	for {
		req, err := c.rconn.ReadRequest()
		if err != nil {
			if err != io.EOF {
				c.log("ERR: %s", err)
			}
			return
		}

		ok := c.handleRequest(req)
		if !ok {
			return
		}
	}
}

func (c *client) writeRes(res *gortsplib.Response) {
	c.rconn.WriteResponse(res)
}

func (c *client) writeResError(req *gortsplib.Request, err error) {
	c.log("ERR: %s", err)

	if cseq, ok := req.Headers["CSeq"]; ok {
		c.rconn.WriteResponse(&gortsplib.Response{
			StatusCode: 400,
			Status:     "Bad Request",
			Headers: map[string]string{
				"CSeq": cseq,
			},
		})
	} else {
		c.rconn.WriteResponse(&gortsplib.Response{
			StatusCode: 400,
			Status:     "Bad Request",
		})
	}
}

func (c *client) handleRequest(req *gortsplib.Request) bool {
	c.log(req.Method)

	cseq, ok := req.Headers["CSeq"]
	if !ok {
		c.writeResError(req, fmt.Errorf("cseq missing"))
		return false
	}

	ur, err := url.Parse(req.Url)
	if err != nil {
		c.writeResError(req, fmt.Errorf("unable to parse path '%s'", req.Url))
		return false
	}

	path := func() string {
		ret := ur.Path

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
	case "OPTIONS":
		// do not check state, since OPTIONS can be requested
		// in any state

		c.writeRes(&gortsplib.Response{
			StatusCode: 200,
			Status:     "OK",
			Headers: map[string]string{
				"CSeq": cseq,
				"Public": strings.Join([]string{
					"DESCRIBE",
					"ANNOUNCE",
					"SETUP",
					"PLAY",
					"PAUSE",
					"RECORD",
					"TEARDOWN",
				}, ", "),
			},
		})
		return true

	case "DESCRIBE":
		if c.state != "STARTING" {
			c.writeResError(req, fmt.Errorf("client is in state '%s'", c.state))
			return false
		}

		sdp, err := func() ([]byte, error) {
			c.p.mutex.RLock()
			defer c.p.mutex.RUnlock()

			pub, ok := c.p.publishers[path]
			if !ok {
				return nil, fmt.Errorf("no one is streaming on path '%s'", path)
			}

			return pub.streamSdpText, nil
		}()
		if err != nil {
			c.writeResError(req, err)
			return false
		}

		c.writeRes(&gortsplib.Response{
			StatusCode: 200,
			Status:     "OK",
			Headers: map[string]string{
				"CSeq":         cseq,
				"Content-Base": req.Url,
				"Content-Type": "application/sdp",
			},
			Content: sdp,
		})
		return true

	case "ANNOUNCE":
		if c.state != "STARTING" {
			c.writeResError(req, fmt.Errorf("client is in state '%s'", c.state))
			return false
		}

		ct, ok := req.Headers["Content-Type"]
		if !ok {
			c.writeResError(req, fmt.Errorf("Content-Type header missing"))
			return false
		}

		if ct != "application/sdp" {
			c.writeResError(req, fmt.Errorf("unsupported Content-Type '%s'", ct))
			return false
		}

		sdpParsed, err := func() (*sdp.Message, error) {
			s, err := sdp.DecodeSession(req.Content, nil)
			if err != nil {
				return nil, err
			}

			m := &sdp.Message{}
			d := sdp.NewDecoder(s)
			err = d.Decode(m)
			if err != nil {
				return nil, err
			}

			return m, nil
		}()
		if err != nil {
			c.writeResError(req, fmt.Errorf("invalid SDP: %s", err))
			return false
		}

		if c.p.publishKey != "" {
			q, err := url.ParseQuery(ur.RawQuery)
			if err != nil {
				c.writeResError(req, fmt.Errorf("unable to parse query"))
				return false
			}

			key, ok := q["key"]
			if !ok || len(key) != 1 || key[0] != c.p.publishKey {
				// reply with 401 and exit
				c.log("ERR: publish key wrong or missing")
				c.writeRes(&gortsplib.Response{
					StatusCode: 401,
					Status:     "Unauthorized",
					Headers: map[string]string{
						"CSeq": req.Headers["CSeq"],
					},
				})
				return false
			}
		}

		err = func() error {
			c.p.mutex.Lock()
			defer c.p.mutex.Unlock()

			_, ok := c.p.publishers[path]
			if ok {
				return fmt.Errorf("another client is already publishing on path '%s'", path)
			}

			c.path = path
			c.p.publishers[path] = c
			c.streamSdpText = req.Content
			c.streamSdpParsed = sdpParsed
			c.state = "ANNOUNCE"
			return nil
		}()
		if err != nil {
			c.writeResError(req, err)
			return false
		}

		c.writeRes(&gortsplib.Response{
			StatusCode: 200,
			Status:     "OK",
			Headers: map[string]string{
				"CSeq": cseq,
			},
		})
		return true

	case "SETUP":
		transportStr, ok := req.Headers["Transport"]
		if !ok {
			c.writeResError(req, fmt.Errorf("transport header missing"))
			return false
		}

		th := newTransportHeader(transportStr)

		if _, ok := th["unicast"]; !ok {
			c.writeResError(req, fmt.Errorf("transport header does not contain unicast"))
			return false
		}

		switch c.state {
		// play
		case "STARTING", "PRE_PLAY":
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
					c.log("ERR: udp streaming is disabled")
					c.rconn.WriteResponse(&gortsplib.Response{
						StatusCode: 461,
						Status:     "Unsupported Transport",
						Headers: map[string]string{
							"CSeq": cseq,
						},
					})
					return false
				}

				rtpPort, rtcpPort := th.getClientPorts()
				if rtpPort == 0 || rtcpPort == 0 {
					c.writeResError(req, fmt.Errorf("transport header does not have valid client ports (%s)", transportStr))
					return false
				}

				if c.path != "" && path != c.path {
					c.writeResError(req, fmt.Errorf("path has changed"))
					return false
				}

				err = func() error {
					c.p.mutex.Lock()
					defer c.p.mutex.Unlock()

					pub, ok := c.p.publishers[path]
					if !ok {
						return fmt.Errorf("no one is streaming on path '%s'", path)
					}

					if len(c.streamTracks) > 0 && c.streamProtocol != _STREAM_PROTOCOL_UDP {
						return fmt.Errorf("client want to send tracks with different protocols")
					}

					if len(c.streamTracks) >= len(pub.streamSdpParsed.Medias) {
						return fmt.Errorf("all the tracks have already been setup")
					}

					c.path = path
					c.streamProtocol = _STREAM_PROTOCOL_UDP
					c.streamTracks = append(c.streamTracks, &track{
						rtpPort:  rtpPort,
						rtcpPort: rtcpPort,
					})

					c.state = "PRE_PLAY"
					return nil
				}()
				if err != nil {
					c.writeResError(req, err)
					return false
				}

				c.writeRes(&gortsplib.Response{
					StatusCode: 200,
					Status:     "OK",
					Headers: map[string]string{
						"CSeq": cseq,
						"Transport": strings.Join([]string{
							"RTP/AVP/UDP",
							"unicast",
							fmt.Sprintf("client_port=%d-%d", rtpPort, rtcpPort),
							fmt.Sprintf("server_port=%d-%d", c.p.rtpPort, c.p.rtcpPort),
						}, ";"),
						"Session": "12345678",
					},
				})
				return true

				// play via TCP
			} else if _, ok := th["RTP/AVP/TCP"]; ok {
				if _, ok := c.p.protocols[_STREAM_PROTOCOL_TCP]; !ok {
					c.log("ERR: tcp streaming is disabled")
					c.rconn.WriteResponse(&gortsplib.Response{
						StatusCode: 461,
						Status:     "Unsupported Transport",
						Headers: map[string]string{
							"CSeq": cseq,
						},
					})
					return false
				}

				if c.path != "" && path != c.path {
					c.writeResError(req, fmt.Errorf("path has changed"))
					return false
				}

				err = func() error {
					c.p.mutex.Lock()
					defer c.p.mutex.Unlock()

					pub, ok := c.p.publishers[path]
					if !ok {
						return fmt.Errorf("no one is streaming on path '%s'", path)
					}

					if len(c.streamTracks) > 0 && c.streamProtocol != _STREAM_PROTOCOL_TCP {
						return fmt.Errorf("client want to send tracks with different protocols")
					}

					if len(c.streamTracks) >= len(pub.streamSdpParsed.Medias) {
						return fmt.Errorf("all the tracks have already been setup")
					}

					c.path = path
					c.streamProtocol = _STREAM_PROTOCOL_TCP
					c.streamTracks = append(c.streamTracks, &track{
						rtpPort:  0,
						rtcpPort: 0,
					})

					c.state = "PRE_PLAY"
					return nil
				}()
				if err != nil {
					c.writeResError(req, err)
					return false
				}

				interleaved := fmt.Sprintf("%d-%d", ((len(c.streamTracks) - 1) * 2), ((len(c.streamTracks)-1)*2)+1)

				c.writeRes(&gortsplib.Response{
					StatusCode: 200,
					Status:     "OK",
					Headers: map[string]string{
						"CSeq": cseq,
						"Transport": strings.Join([]string{
							"RTP/AVP/TCP",
							"unicast",
							fmt.Sprintf("interleaved=%s", interleaved),
						}, ";"),
						"Session": "12345678",
					},
				})
				return true

			} else {
				c.writeResError(req, fmt.Errorf("transport header does not contain a valid protocol (RTP/AVP, RTP/AVP/UDP or RTP/AVP/TCP) (%s)", transportStr))
				return false
			}

		// record
		case "ANNOUNCE", "PRE_RECORD":
			if _, ok := th["mode=record"]; !ok {
				c.writeResError(req, fmt.Errorf("transport header does not contain mode=record"))
				return false
			}

			if path != c.path {
				c.writeResError(req, fmt.Errorf("path has changed"))
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
					c.log("ERR: udp streaming is disabled")
					c.rconn.WriteResponse(&gortsplib.Response{
						StatusCode: 461,
						Status:     "Unsupported Transport",
						Headers: map[string]string{
							"CSeq": cseq,
						},
					})
					return false
				}

				rtpPort, rtcpPort := th.getClientPorts()
				if rtpPort == 0 || rtcpPort == 0 {
					c.writeResError(req, fmt.Errorf("transport header does not have valid client ports (%s)", transportStr))
					return false
				}

				err = func() error {
					c.p.mutex.Lock()
					defer c.p.mutex.Unlock()

					if len(c.streamTracks) > 0 && c.streamProtocol != _STREAM_PROTOCOL_UDP {
						return fmt.Errorf("client want to send tracks with different protocols")
					}

					if len(c.streamTracks) >= len(c.streamSdpParsed.Medias) {
						return fmt.Errorf("all the tracks have already been setup")
					}

					c.streamProtocol = _STREAM_PROTOCOL_UDP
					c.streamTracks = append(c.streamTracks, &track{
						rtpPort:  rtpPort,
						rtcpPort: rtcpPort,
					})

					c.state = "PRE_RECORD"
					return nil
				}()
				if err != nil {
					c.writeResError(req, err)
					return false
				}

				c.writeRes(&gortsplib.Response{
					StatusCode: 200,
					Status:     "OK",
					Headers: map[string]string{
						"CSeq": cseq,
						"Transport": strings.Join([]string{
							"RTP/AVP/UDP",
							"unicast",
							fmt.Sprintf("client_port=%d-%d", rtpPort, rtcpPort),
							fmt.Sprintf("server_port=%d-%d", c.p.rtpPort, c.p.rtcpPort),
						}, ";"),
						"Session": "12345678",
					},
				})
				return true

				// record via TCP
			} else if _, ok := th["RTP/AVP/TCP"]; ok {
				if _, ok := c.p.protocols[_STREAM_PROTOCOL_TCP]; !ok {
					c.log("ERR: tcp streaming is disabled")
					c.rconn.WriteResponse(&gortsplib.Response{
						StatusCode: 461,
						Status:     "Unsupported Transport",
						Headers: map[string]string{
							"CSeq": cseq,
						},
					})
					return false
				}

				var interleaved string
				err = func() error {
					c.p.mutex.Lock()
					defer c.p.mutex.Unlock()

					if len(c.streamTracks) > 0 && c.streamProtocol != _STREAM_PROTOCOL_TCP {
						return fmt.Errorf("client want to send tracks with different protocols")
					}

					if len(c.streamTracks) >= len(c.streamSdpParsed.Medias) {
						return fmt.Errorf("all the tracks have already been setup")
					}

					interleaved = th.getKeyValue("interleaved")
					if interleaved == "" {
						return fmt.Errorf("transport header does not contain interleaved field")
					}

					expInterleaved := fmt.Sprintf("%d-%d", 0+len(c.streamTracks)*2, 1+len(c.streamTracks)*2)
					if interleaved != expInterleaved {
						return fmt.Errorf("wrong interleaved value, expected '%s', got '%s'", expInterleaved, interleaved)
					}

					c.streamProtocol = _STREAM_PROTOCOL_TCP
					c.streamTracks = append(c.streamTracks, &track{
						rtpPort:  0,
						rtcpPort: 0,
					})

					c.state = "PRE_RECORD"
					return nil
				}()
				if err != nil {
					c.writeResError(req, err)
					return false
				}

				c.writeRes(&gortsplib.Response{
					StatusCode: 200,
					Status:     "OK",
					Headers: map[string]string{
						"CSeq": cseq,
						"Transport": strings.Join([]string{
							"RTP/AVP/TCP",
							"unicast",
							fmt.Sprintf("interleaved=%s", interleaved),
						}, ";"),
						"Session": "12345678",
					},
				})
				return true

			} else {
				c.writeResError(req, fmt.Errorf("transport header does not contain a valid protocol (RTP/AVP, RTP/AVP/UDP or RTP/AVP/TCP) (%s)", transportStr))
				return false
			}

		default:
			c.writeResError(req, fmt.Errorf("client is in state '%s'", c.state))
			return false
		}

	case "PLAY":
		if c.state != "PRE_PLAY" {
			c.writeResError(req, fmt.Errorf("client is in state '%s'", c.state))
			return false
		}

		if path != c.path {
			c.writeResError(req, fmt.Errorf("path has changed"))
			return false
		}

		err := func() error {
			c.p.mutex.Lock()
			defer c.p.mutex.Unlock()

			pub, ok := c.p.publishers[c.path]
			if !ok {
				return fmt.Errorf("no one is streaming on path '%s'", c.path)
			}

			if len(c.streamTracks) != len(pub.streamSdpParsed.Medias) {
				return fmt.Errorf("not all tracks have been setup")
			}

			return nil
		}()
		if err != nil {
			c.writeResError(req, err)
			return false
		}

		// first write response, then set state
		// otherwise, in case of TCP connections, RTP packets could be written
		// before the response
		c.writeRes(&gortsplib.Response{
			StatusCode: 200,
			Status:     "OK",
			Headers: map[string]string{
				"CSeq":    cseq,
				"Session": "12345678",
			},
		})

		c.log("is receiving on path '%s', %d %s via %s", c.path, len(c.streamTracks), func() string {
			if len(c.streamTracks) == 1 {
				return "track"
			}
			return "tracks"
		}(), c.streamProtocol)

		c.p.mutex.Lock()
		c.state = "PLAY"
		c.p.mutex.Unlock()

		// when protocol is TCP, the RTSP connection becomes a RTP connection
		// receive RTP feedback, do not parse it, wait until connection closes
		if c.streamProtocol == _STREAM_PROTOCOL_TCP {
			buf := make([]byte, 2048)
			for {
				_, err := c.rconn.Read(buf)
				if err != nil {
					if err != io.EOF {
						c.log("ERR: %s", err)
					}
					return false
				}
			}
		}

		return true

	case "PAUSE":
		if c.state != "PLAY" {
			c.writeResError(req, fmt.Errorf("client is in state '%s'", c.state))
			return false
		}

		if path != c.path {
			c.writeResError(req, fmt.Errorf("path has changed"))
			return false
		}

		c.log("paused")

		c.p.mutex.Lock()
		c.state = "PRE_PLAY"
		c.p.mutex.Unlock()

		c.writeRes(&gortsplib.Response{
			StatusCode: 200,
			Status:     "OK",
			Headers: map[string]string{
				"CSeq":    cseq,
				"Session": "12345678",
			},
		})
		return true

	case "RECORD":
		if c.state != "PRE_RECORD" {
			c.writeResError(req, fmt.Errorf("client is in state '%s'", c.state))
			return false
		}

		if path != c.path {
			c.writeResError(req, fmt.Errorf("path has changed"))
			return false
		}

		err := func() error {
			c.p.mutex.Lock()
			defer c.p.mutex.Unlock()

			if len(c.streamTracks) != len(c.streamSdpParsed.Medias) {
				return fmt.Errorf("not all tracks have been setup")
			}

			return nil
		}()
		if err != nil {
			c.writeResError(req, err)
			return false
		}

		c.writeRes(&gortsplib.Response{
			StatusCode: 200,
			Status:     "OK",
			Headers: map[string]string{
				"CSeq":    cseq,
				"Session": "12345678",
			},
		})

		c.p.mutex.Lock()
		c.state = "RECORD"
		c.p.mutex.Unlock()

		c.log("is publishing on path '%s', %d %s via %s", c.path, len(c.streamTracks), func() string {
			if len(c.streamTracks) == 1 {
				return "track"
			}
			return "tracks"
		}(), c.streamProtocol)

		// when protocol is TCP, the RTSP connection becomes a RTP connection
		// receive RTP data and parse it
		if c.streamProtocol == _STREAM_PROTOCOL_TCP {
			buf := make([]byte, 2048)
			for {
				channel, n, err := c.rconn.ReadInterleavedFrame(buf)
				if err != nil {
					if _, ok := err.(*net.OpError); ok {
					} else if err == io.EOF {
					} else {
						c.log("ERR: %s", err)
					}
					return false
				}

				trackId, trackFlow := interleavedChannelToTrack(channel)

				if trackId >= len(c.streamTracks) {
					c.log("ERR: invalid track id '%d'", trackId)
					return false
				}

				c.p.mutex.RLock()
				c.p.forwardTrack(c.path, trackId, trackFlow, buf[:n])
				c.p.mutex.RUnlock()
			}
		}

		return true

	case "TEARDOWN":
		// close connection silently
		return false

	default:
		c.writeResError(req, fmt.Errorf("unhandled method '%s'", req.Method))
		return false
	}
}
