package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"

	"rtsp-server/rtsp"

	"gortc.io/sdp"
)

var (
	errTeardown = errors.New("teardown")
	errPlay     = errors.New("play")
	errRecord   = errors.New("record")
)

func interleavedChannelToTrack(channel int) (trackFlow, int) {
	if (channel % 2) == 0 {
		return _TRACK_FLOW_RTP, (channel / 2)
	}
	return _TRACK_FLOW_RTCP, ((channel - 1) / 2)
}

func trackToInterleavedChannel(flow trackFlow, id int) int {
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
	rconn           *rtsp.Conn
	state           string
	ip              net.IP
	streamSdpText   []byte       // filled only if publisher
	streamSdpParsed *sdp.Message // filled only if publisher
	streamProtocol  streamProtocol
	streamTracks    []*track
}

func newRtspClient(p *program, nconn net.Conn) *client {
	c := &client{
		p:     p,
		rconn: rtsp.NewConn(nconn),
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

	if c.p.publisher == c {
		c.p.publisher = nil

		// if the publisher has disconnected
		// close all other connections
		for oc := range c.p.clients {
			oc.close()
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

		c.log(req.Method)

		res, err := c.handleRequest(req)

		switch err {
		// normal response
		case nil:
			err = c.rconn.WriteResponse(res)
			if err != nil {
				c.log("ERR: %s", err)
				return
			}

		// TEARDOWN, close connection silently
		case errTeardown:
			return

		// PLAY: first write response, then set state
		// otherwise, in case of TCP connections, RTP packets could be written
		// before the response
		// then switch to RTP if TCP
		case errPlay:
			err = c.rconn.WriteResponse(res)
			if err != nil {
				c.log("ERR: %s", err)
				return
			}

			c.log("is receiving %d %s via %s", len(c.streamTracks), func() string {
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
						return
					}
				}
			}

		// RECORD: switch to RTP if TCP
		case errRecord:
			err = c.rconn.WriteResponse(res)
			if err != nil {
				c.log("ERR: %s", err)
				return
			}

			c.p.mutex.Lock()
			c.state = "RECORD"
			c.p.mutex.Unlock()

			c.log("is publishing %d %s via %s", len(c.streamTracks), func() string {
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
						return
					}

					trackFlow, trackId := interleavedChannelToTrack(channel)

					if trackId >= len(c.streamTracks) {
						c.log("ERR: invalid track id '%d'", trackId)
						return
					}

					c.p.mutex.RLock()
					c.p.forwardTrack(trackFlow, trackId, buf[:n])
					c.p.mutex.RUnlock()
				}
			}

		// error: write and exit
		default:
			c.log("ERR: %s", err)

			if cseq, ok := req.Headers["CSeq"]; ok {
				c.rconn.WriteResponse(&rtsp.Response{
					StatusCode: 400,
					Status:     "Bad Request",
					Headers: map[string]string{
						"CSeq": cseq,
					},
				})
			} else {
				c.rconn.WriteResponse(&rtsp.Response{
					StatusCode: 400,
					Status:     "Bad Request",
				})
			}
			return
		}
	}
}

func (c *client) handleRequest(req *rtsp.Request) (*rtsp.Response, error) {
	cseq, ok := req.Headers["CSeq"]
	if !ok {
		return nil, fmt.Errorf("cseq missing")
	}

	ur, err := url.Parse(req.Path)
	if err != nil {
		return nil, fmt.Errorf("unable to parse path '%s'", req.Path)
	}

	switch req.Method {
	case "OPTIONS":
		// do not check state, since OPTIONS can be requested
		// in any state

		return &rtsp.Response{
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
		}, nil

	case "DESCRIBE":
		if c.state != "STARTING" {
			return nil, fmt.Errorf("client is in state '%s'", c.state)
		}

		sdp, err := func() ([]byte, error) {
			c.p.mutex.RLock()
			defer c.p.mutex.RUnlock()

			if c.p.publisher == nil {
				return nil, fmt.Errorf("no one is streaming")
			}

			return c.p.publisher.streamSdpText, nil
		}()
		if err != nil {
			return nil, err
		}

		return &rtsp.Response{
			StatusCode: 200,
			Status:     "OK",
			Headers: map[string]string{
				"CSeq":         cseq,
				"Content-Base": ur.String(),
				"Content-Type": "application/sdp",
			},
			Content: sdp,
		}, nil

	case "ANNOUNCE":
		if c.state != "STARTING" {
			return nil, fmt.Errorf("client is in state '%s'", c.state)
		}

		ct, ok := req.Headers["Content-Type"]
		if !ok {
			return nil, fmt.Errorf("Content-Type header missing")
		}

		if ct != "application/sdp" {
			return nil, fmt.Errorf("unsupported Content-Type '%s'", ct)
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
			return nil, fmt.Errorf("invalid SDP: %s", err)
		}

		err = func() error {
			c.p.mutex.Lock()
			defer c.p.mutex.Unlock()

			if c.p.publisher != nil {
				return fmt.Errorf("another client is already streaming")
			}

			c.p.publisher = c
			c.streamSdpText = req.Content
			c.streamSdpParsed = sdpParsed
			c.state = "ANNOUNCE"
			return nil
		}()
		if err != nil {
			return nil, err
		}

		return &rtsp.Response{
			StatusCode: 200,
			Status:     "OK",
			Headers: map[string]string{
				"CSeq": cseq,
			},
		}, nil

	case "SETUP":
		transportstr, ok := req.Headers["Transport"]
		if !ok {
			return nil, fmt.Errorf("transport header missing")
		}

		th := newTransportHeader(transportstr)

		if _, ok := th["unicast"]; !ok {
			return nil, fmt.Errorf("transport header does not contain unicast")
		}

		switch c.state {
		// play
		case "STARTING", "PRE_PLAY":
			err := func() error {
				c.p.mutex.RLock()
				defer c.p.mutex.RUnlock()

				if c.p.publisher == nil {
					return fmt.Errorf("no one is streaming")
				}

				return nil
			}()
			if err != nil {
				return nil, err
			}

			// play via UDP
			if _, ok := th["RTP/AVP"]; ok {
				rtpPort, rtcpPort := th.getClientPorts()
				if rtpPort == 0 || rtcpPort == 0 {
					return nil, fmt.Errorf("transport header does not have valid client ports (%s)", transportstr)
				}

				err = func() error {
					c.p.mutex.Lock()
					defer c.p.mutex.Unlock()

					if len(c.streamTracks) > 0 && c.streamProtocol != _STREAM_PROTOCOL_UDP {
						return fmt.Errorf("client want to send tracks with different protocols")
					}

					if len(c.streamTracks) >= len(c.p.publisher.streamSdpParsed.Medias) {
						return fmt.Errorf("all the tracks have already been setup")
					}

					c.streamProtocol = _STREAM_PROTOCOL_UDP
					c.streamTracks = append(c.streamTracks, &track{
						rtpPort:  rtpPort,
						rtcpPort: rtcpPort,
					})

					c.state = "PRE_PLAY"
					return nil
				}()
				if err != nil {
					return nil, err
				}

				return &rtsp.Response{
					StatusCode: 200,
					Status:     "OK",
					Headers: map[string]string{
						"CSeq": cseq,
						"Transport": strings.Join([]string{
							"RTP/AVP",
							"unicast",
							fmt.Sprintf("client_port=%d-%d", rtpPort, rtcpPort),
							// use two fake server ports, since we do not want to receive feedback
							// from the client
							fmt.Sprintf("server_port=%d-%d", c.p.rtpPort+2, c.p.rtcpPort+2),
							"ssrc=1234ABCD",
						}, ";"),
						"Session": "12345678",
					},
				}, nil

				// play via TCP
			} else if _, ok := th["RTP/AVP/TCP"]; ok {
				err = func() error {
					c.p.mutex.Lock()
					defer c.p.mutex.Unlock()

					if len(c.streamTracks) > 0 && c.streamProtocol != _STREAM_PROTOCOL_TCP {
						return fmt.Errorf("client want to send tracks with different protocols")
					}

					if len(c.streamTracks) >= len(c.p.publisher.streamSdpParsed.Medias) {
						return fmt.Errorf("all the tracks have already been setup")
					}

					c.streamProtocol = _STREAM_PROTOCOL_TCP
					c.streamTracks = append(c.streamTracks, &track{
						rtpPort:  0,
						rtcpPort: 0,
					})

					c.state = "PRE_PLAY"
					return nil
				}()
				if err != nil {
					return nil, err
				}

				interleaved := fmt.Sprintf("%d-%d", ((len(c.streamTracks) - 1) * 2), ((len(c.streamTracks)-1)*2)+1)

				return &rtsp.Response{
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
				}, nil

			} else {
				return nil, fmt.Errorf("transport header does not contain a valid protocol (RTP/AVP or RTP/AVP/TCP) (%s)", transportstr)
			}

		// record
		case "ANNOUNCE", "PRE_RECORD":
			if _, ok := th["mode=record"]; !ok {
				return nil, fmt.Errorf("transport header does not contain mode=record")
			}

			// record via UDP
			if _, ok := th["RTP/AVP/UDP"]; ok {
				rtpPort, rtcpPort := th.getClientPorts()
				if rtpPort == 0 || rtcpPort == 0 {
					return nil, fmt.Errorf("transport header does not have valid client ports (%s)", transportstr)
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
					return nil, err
				}

				return &rtsp.Response{
					StatusCode: 200,
					Status:     "OK",
					Headers: map[string]string{
						"CSeq": cseq,
						"Transport": strings.Join([]string{
							"RTP/AVP",
							"unicast",
							fmt.Sprintf("client_port=%d-%d", rtpPort, rtcpPort),
							fmt.Sprintf("server_port=%d-%d", c.p.rtpPort, c.p.rtcpPort),
							"ssrc=1234ABCD",
						}, ";"),
						"Session": "12345678",
					},
				}, nil

				// record via TCP
			} else if _, ok := th["RTP/AVP/TCP"]; ok {
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
					return nil, err
				}

				return &rtsp.Response{
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
				}, nil

			} else {
				return nil, fmt.Errorf("transport header does not contain a valid protocol (RTP/AVP or RTP/AVP/TCP) (%s)", transportstr)
			}

		default:
			return nil, fmt.Errorf("client is in state '%s'", c.state)
		}

	case "PLAY":
		if c.state != "PRE_PLAY" {
			return nil, fmt.Errorf("client is in state '%s'", c.state)
		}

		err := func() error {
			c.p.mutex.Lock()
			defer c.p.mutex.Unlock()

			if len(c.streamTracks) != len(c.p.publisher.streamSdpParsed.Medias) {
				return fmt.Errorf("not all tracks have been setup")
			}

			return nil
		}()
		if err != nil {
			return nil, err
		}

		return &rtsp.Response{
			StatusCode: 200,
			Status:     "OK",
			Headers: map[string]string{
				"CSeq":    cseq,
				"Session": "12345678",
			},
		}, errPlay

	case "PAUSE":
		if c.state != "PLAY" {
			return nil, fmt.Errorf("client is in state '%s'", c.state)
		}

		c.log("paused")

		c.p.mutex.Lock()
		c.state = "PRE_PLAY"
		c.p.mutex.Unlock()

		return &rtsp.Response{
			StatusCode: 200,
			Status:     "OK",
			Headers: map[string]string{
				"CSeq":    cseq,
				"Session": "12345678",
			},
		}, nil

	case "RECORD":
		if c.state != "PRE_RECORD" {
			return nil, fmt.Errorf("client is in state '%s'", c.state)
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
			return nil, err
		}

		return &rtsp.Response{
			StatusCode: 200,
			Status:     "OK",
			Headers: map[string]string{
				"CSeq":    cseq,
				"Session": "12345678",
			},
		}, errRecord

	case "TEARDOWN":
		return nil, errTeardown

	default:
		return nil, fmt.Errorf("unhandled method '%s'", req.Method)
	}
}
