package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"rtsp-server/rtsp"
)

type rtspClient struct {
	p        *program
	rconn    *rtsp.Conn
	state    string
	IP       net.IP
	rtpPort  int
	rtcpPort int
}

func newRtspClient(p *program, rconn *rtsp.Conn) *rtspClient {
	c := &rtspClient{
		p:     p,
		rconn: rconn,
		state: "STARTING",
	}

	c.p.mutex.Lock()
	c.p.clients[c] = struct{}{}
	c.p.mutex.Unlock()

	return c
}

func (c *rtspClient) close() error {
	delete(c.p.clients, c)

	if c.p.streamAuthor == c {
		c.p.streamAuthor = nil
		c.p.streamSdp = nil

		// if the streamer has disconnected
		// close all other connections
		for oc := range c.p.clients {
			oc.close()
		}
	}

	return c.rconn.Close()
}

func (c *rtspClient) log(format string, args ...interface{}) {
	format = "[RTSP client " + c.rconn.RemoteAddr().String() + "] " + format
	log.Printf(format, args...)
}

func (c *rtspClient) run(wg sync.WaitGroup) {
	defer wg.Done()
	defer c.log("disconnected")
	defer func() {
		c.p.mutex.Lock()
		defer c.p.mutex.Unlock()
		c.close()
	}()

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

		cseq, ok := req.Headers["CSeq"]
		if !ok {
			c.log("ERR: cseq missing")
			return
		}

		ur, err := url.Parse(req.Path)
		if err != nil {
			c.log("ERR: unable to parse path '%s'", req.Path)
			return
		}

		switch req.Method {
		case "OPTIONS":
			// do not check state, since OPTIONS can be requested
			// in any state

			err = c.rconn.WriteResponse(&rtsp.Response{
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
			if err != nil {
				c.log("ERR: %s", err)
				return
			}

		case "DESCRIBE":
			if c.state != "STARTING" {
				c.log("ERR: client is in state '%s'", c.state)
				return
			}

			sdp, err := func() ([]byte, error) {
				c.p.mutex.Lock()
				defer c.p.mutex.Unlock()

				if len(c.p.streamSdp) == 0 {
					return nil, fmt.Errorf("no one is streaming")
				}

				return c.p.streamSdp, nil
			}()
			if err != nil {
				c.log("ERR: %s", err)
				return
			}

			err = c.rconn.WriteResponse(&rtsp.Response{
				StatusCode: 200,
				Status:     "OK",
				Headers: map[string]string{
					"CSeq":         cseq,
					"Content-Base": ur.String(),
					"Content-Type": "application/sdp",
				},
				Content: sdp,
			})
			if err != nil {
				c.log("ERR: %s", err)
				return
			}

		case "ANNOUNCE":
			if c.state != "STARTING" {
				c.log("ERR: client is in state '%s'", c.state)
				return
			}

			ct, ok := req.Headers["Content-Type"]
			if !ok {
				c.log("ERR: Content-Type header missing")
				return
			}

			if ct != "application/sdp" {
				c.log("ERR: unsupported Content-Type '%s'", ct)
				return
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
				c.log("ERR: %s", err)
				return
			}

			err = c.rconn.WriteResponse(&rtsp.Response{
				StatusCode: 200,
				Status:     "OK",
				Headers: map[string]string{
					"CSeq": cseq,
				},
			})
			if err != nil {
				c.log("ERR: %s", err)
				return
			}

			c.p.mutex.Lock()
			c.state = "ANNOUNCE"
			c.p.mutex.Unlock()

		case "SETUP":
			transport, ok := req.Headers["Transport"]
			if !ok {
				c.log("ERR: transport header missing")
				return
			}

			transports := strings.Split(transport, ";")

			ok = func() bool {
				for _, t := range transports {
					if t == "unicast" {
						return true
					}
				}
				return false
			}()
			if !ok {
				c.log("ERR: transport header does not contain unicast")
				return
			}

			clientPort1, clientPort2 := func() (int, int) {
				for _, t := range transports {
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
			}()
			if clientPort1 == 0 || clientPort2 == 0 {
				c.log("ERR: transport header does not have valid client ports (%s)", transport)
				return
			}

			switch c.state {
			// play
			case "STARTING":
				ok = func() bool {
					for _, t := range transports {
						if t == "RTP/AVP" {
							return true
						}
					}
					return false
				}()
				if !ok {
					c.log("ERR: transport header does not contain RTP/AVP")
					return
				}

				err = c.rconn.WriteResponse(&rtsp.Response{
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
				})
				if err != nil {
					c.log("ERR: %s", err)
					return
				}

				c.p.mutex.Lock()
				c.rtpPort = clientPort1
				c.rtcpPort = clientPort2
				c.state = "PRE_PLAY"
				c.p.mutex.Unlock()

			// record
			case "ANNOUNCE":
				ok = func() bool {
					for _, t := range transports {
						if t == "RTP/AVP/UDP" {
							return true
						}
					}
					return false
				}()
				if !ok {
					c.log("ERR: transport header does not contain RTP/AVP/UDP")
					return
				}

				ok = func() bool {
					for _, t := range transports {
						if t == "mode=record" {
							return true
						}
					}
					return false
				}()
				if !ok {
					c.log("ERR: transport header does not contain mode=record")
					return
				}

				err = c.rconn.WriteResponse(&rtsp.Response{
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
				})
				if err != nil {
					c.log("ERR: %s", err)
					return
				}

				c.p.mutex.Lock()
				ipstr, _, _ := net.SplitHostPort(c.rconn.RemoteAddr().String())
				c.IP = net.ParseIP(ipstr)
				c.rtpPort = clientPort1
				c.rtcpPort = clientPort2
				c.state = "PRE_RECORD"
				c.p.mutex.Unlock()

			default:
				c.log("ERR: client is in state '%s'", c.state)
				return
			}

		case "PLAY":
			if c.state != "PRE_PLAY" {
				c.log("ERR: client is in state '%s'", c.state)
				return
			}

			err = c.rconn.WriteResponse(&rtsp.Response{
				StatusCode: 200,
				Status:     "OK",
				Headers: map[string]string{
					"CSeq":    cseq,
					"Session": "12345678",
				},
			})
			if err != nil {
				c.log("ERR: %s", err)
				return
			}

			c.p.mutex.Lock()
			c.state = "PLAY"
			c.p.mutex.Unlock()

		case "PAUSE":
			if c.state != "PLAY" {
				c.log("ERR: client is in state '%s'", c.state)
				return
			}

			c.p.mutex.Lock()
			c.state = "PRE_PLAY"
			c.p.mutex.Unlock()

		case "RECORD":
			if c.state != "PRE_RECORD" {
				c.log("ERR: client is in state '%s'", c.state)
				return
			}

			err = c.rconn.WriteResponse(&rtsp.Response{
				StatusCode: 200,
				Status:     "OK",
				Headers: map[string]string{
					"CSeq":    cseq,
					"Session": "12345678",
				},
			})
			if err != nil {
				c.log("ERR: %s", err)
				return
			}

			c.p.mutex.Lock()
			c.state = "RECORD"
			c.p.mutex.Unlock()

		case "TEARDOWN":
			return

		default:
			c.log("ERR: method %s unhandled", req.Method)
		}
	}
}
