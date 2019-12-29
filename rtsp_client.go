package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"

	"rtsp-server/rtsp"
)

var (
	errTeardown = errors.New("teardown")
	errPlay     = errors.New("play")
	errRecord   = errors.New("record")
)

type rtspClient struct {
	p        *program
	nconn    net.Conn
	state    string
	IP       net.IP
	rtpProto string
	rtpPort  int
	rtcpPort int
}

func newRtspClient(p *program, nconn net.Conn) *rtspClient {
	c := &rtspClient{
		p:     p,
		nconn: nconn,
		state: "STARTING",
	}

	c.p.mutex.Lock()
	c.p.clients[c] = struct{}{}
	c.p.mutex.Unlock()

	return c
}

func (c *rtspClient) close() error {
	// already deleted
	if _, ok := c.p.clients[c]; !ok {
		return nil
	}

	delete(c.p.clients, c)
	c.nconn.Close()

	if c.p.streamAuthor == c {
		c.p.streamAuthor = nil
		c.p.streamSdp = nil

		// if the publisher has disconnected
		// close all other connections
		for oc := range c.p.clients {
			oc.close()
		}
	}

	return nil
}

func (c *rtspClient) log(format string, args ...interface{}) {
	format = "[RTSP client " + c.nconn.RemoteAddr().String() + "] " + format
	log.Printf(format, args...)
}

func (c *rtspClient) run() {
	defer c.log("disconnected")
	defer func() {
		c.p.mutex.Lock()
		defer c.p.mutex.Unlock()
		c.close()
	}()

	ipstr, _, _ := net.SplitHostPort(c.nconn.RemoteAddr().String())
	c.IP = net.ParseIP(ipstr)

	rconn := &rtsp.Conn{c.nconn}

	c.log("connected")

	for {
		req, err := rconn.ReadRequest()
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
			err = rconn.WriteResponse(res)
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
			err = rconn.WriteResponse(res)
			if err != nil {
				c.log("ERR: %s", err)
				return
			}

			c.log("is receiving (via %s)", c.rtpProto)

			c.p.mutex.Lock()
			c.state = "PLAY"
			c.p.mutex.Unlock()

			// when rtp protocol is TCP, the RTSP connection becomes a RTP connection
			// receive RTP feedback, do not parse it, wait until connection closes
			if c.rtpProto == "tcp" {
				buf := make([]byte, 1024)
				for {
					_, err := c.nconn.Read(buf)
					if err != nil {
						return
					}
				}
			}

		// RECORD: switch to RTP if TCP
		case errRecord:
			err = rconn.WriteResponse(res)
			if err != nil {
				c.log("ERR: %s", err)
				return
			}

			c.p.mutex.Lock()
			c.state = "RECORD"
			c.p.mutex.Unlock()

			c.log("is publishing (via %s)", c.rtpProto)

			// when rtp protocol is TCP, the RTSP connection becomes a RTP connection
			// receive RTP data and parse it
			if c.rtpProto == "tcp" {
				packet := make([]byte, 2048)
				bconn := bufio.NewReader(c.nconn)
				for {
					byts, err := bconn.Peek(4)
					if err != nil {
						return
					}
					bconn.Discard(4)

					if byts[0] != 0x24 {
						c.log("ERR: wrong magic byte")
						return
					}

					if byts[1] != 0x00 {
						c.log("ERR: wrong channel")
						return
					}

					plen := binary.BigEndian.Uint16(byts[2:])
					if plen > 2048 {
						c.log("ERR: packet len > 2048")
						return
					}

					_, err = io.ReadFull(bconn, packet[:plen])
					if err != nil {
						return
					}

					c.p.handleRtp(packet[:plen])
				}
			}

		// error: write and exit
		default:
			c.log("ERR: %s", err)

			if cseq, ok := req.Headers["cseq"]; ok {
				rconn.WriteResponse(&rtsp.Response{
					StatusCode: 400,
					Status:     "Bad Request",
					Headers: map[string]string{
						"CSeq": cseq,
					},
				})
			} else {
				rconn.WriteResponse(&rtsp.Response{
					StatusCode: 400,
					Status:     "Bad Request",
				})
			}
			return
		}
	}
}

func (c *rtspClient) handleRequest(req *rtsp.Request) (*rtsp.Response, error) {
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

			if len(c.p.streamSdp) == 0 {
				return nil, fmt.Errorf("no one is streaming")
			}

			return c.p.streamSdp, nil
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

		err := func() error {
			c.p.mutex.Lock()
			defer c.p.mutex.Unlock()

			if c.p.streamAuthor != nil {
				return fmt.Errorf("another client is already streaming")
			}

			c.p.streamAuthor = c
			c.p.streamSdp = req.Content
			return nil
		}()
		if err != nil {
			return nil, err
		}

		c.p.mutex.Lock()
		c.state = "ANNOUNCE"
		c.p.mutex.Unlock()

		return &rtsp.Response{
			StatusCode: 200,
			Status:     "OK",
			Headers: map[string]string{
				"CSeq": cseq,
			},
		}, nil

	case "SETUP":
		transport, ok := req.Headers["Transport"]
		if !ok {
			return nil, fmt.Errorf("transport header missing")
		}

		transports := make(map[string]struct{})
		for _, t := range strings.Split(transport, ";") {
			transports[t] = struct{}{}
		}

		if _, ok := transports["unicast"]; !ok {
			return nil, fmt.Errorf("transport header does not contain unicast")
		}

		getPorts := func() (int, int) {
			for t := range transports {
				if !strings.HasPrefix(t, "client_port=") {
					continue
				}
				t = t[len("client_port="):]

				ports := strings.Split(t, "-")
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
			return 0, 0
		}

		switch c.state {
		// play
		case "STARTING":
			// UDP
			if _, ok := transports["RTP/AVP"]; ok {
				clientPort1, clientPort2 := getPorts()
				if clientPort1 == 0 || clientPort2 == 0 {
					return nil, fmt.Errorf("transport header does not have valid client ports (%s)", transport)
				}

				c.p.mutex.Lock()
				c.rtpProto = "udp"
				c.rtpPort = clientPort1
				c.rtcpPort = clientPort2
				c.state = "PRE_PLAY"
				c.p.mutex.Unlock()

				return &rtsp.Response{
					StatusCode: 200,
					Status:     "OK",
					Headers: map[string]string{
						"CSeq": cseq,
						"Transport": strings.Join([]string{
							"RTP/AVP",
							"unicast",
							fmt.Sprintf("client_port=%d-%d", clientPort1, clientPort2),
							// use two fake server ports, since we do not want to receive feedback
							// from the client
							fmt.Sprintf("server_port=%d-%d", c.p.rtpPort+2, c.p.rtcpPort+2),
							"ssrc=1234ABCD",
						}, ";"),
						"Session": "12345678",
					},
				}, nil

				// TCP
			} else if _, ok := transports["RTP/AVP/TCP"]; ok {
				c.p.mutex.Lock()
				c.rtpProto = "tcp"
				c.state = "PRE_PLAY"
				c.p.mutex.Unlock()

				return &rtsp.Response{
					StatusCode: 200,
					Status:     "OK",
					Headers: map[string]string{
						"CSeq": cseq,
						"Transport": strings.Join([]string{
							"RTP/AVP/TCP",
							"unicast",
							"destination=127.0.0.1",
							"source=127.0.0.1",
							"interleaved=0-1",
						}, ";"),
						"Session": "12345678",
					},
				}, nil

			} else {
				return nil, fmt.Errorf("transport header does not contain a valid protocol (RTP/AVP or RTP/AVP/TCP) (%s)", transport)
			}

		// record
		case "ANNOUNCE":
			if _, ok := transports["mode=record"]; !ok {
				return nil, fmt.Errorf("transport header does not contain mode=record")
			}

			if _, ok := transports["RTP/AVP/UDP"]; ok {
				clientPort1, clientPort2 := getPorts()
				if clientPort1 == 0 || clientPort2 == 0 {
					return nil, fmt.Errorf("transport header does not have valid client ports (%s)", transport)
				}

				c.p.mutex.Lock()
				c.rtpProto = "udp"
				c.rtpPort = clientPort1
				c.rtcpPort = clientPort2
				c.state = "PRE_RECORD"
				c.p.mutex.Unlock()

				return &rtsp.Response{
					StatusCode: 200,
					Status:     "OK",
					Headers: map[string]string{
						"CSeq": cseq,
						"Transport": strings.Join([]string{
							"RTP/AVP",
							"unicast",
							fmt.Sprintf("client_port=%d-%d", clientPort1, clientPort2),
							fmt.Sprintf("server_port=%d-%d", c.p.rtpPort, c.p.rtcpPort),
							"ssrc=1234ABCD",
						}, ";"),
						"Session": "12345678",
					},
				}, nil

			} else if _, ok := transports["RTP/AVP/TCP"]; ok {
				c.p.mutex.Lock()
				c.rtpProto = "tcp"
				c.state = "PRE_RECORD"
				c.p.mutex.Unlock()

				return &rtsp.Response{
					StatusCode: 200,
					Status:     "OK",
					Headers: map[string]string{
						"CSeq": cseq,
						"Transport": strings.Join([]string{
							"RTP/AVP/TCP",
							"unicast",
							"destination=127.0.0.1",
							"source=127.0.0.1",
						}, ";"),
						"Session": "12345678",
					},
				}, nil

			} else {
				return nil, fmt.Errorf("transport header does not contain a valid protocol (RTP/AVP or RTP/AVP/TCP) (%s)", transport)
			}

		default:
			return nil, fmt.Errorf("client is in state '%s'", c.state)
		}

	case "PLAY":
		if c.state != "PRE_PLAY" {
			return nil, fmt.Errorf("client is in state '%s'", c.state)
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

		c.log("paused receiving")

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
