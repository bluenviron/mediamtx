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
	pathConf *pathConf
}

type clientAnnounceReq struct {
	res        chan error
	client     *client
	pathName   string
	pathConf   *pathConf
	trackCount int
	sdp        []byte
}

type clientSetupPlayReq struct {
	res      chan error
	client   *client
	pathName string
	trackId  int
}

type readRequestPair struct {
	req *gortsplib.Request
	res chan error
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
	p                 *program
	conn              *gortsplib.ConnServer
	state             clientState
	path              *path
	authUser          string
	authPass          string
	authHelper        *gortsplib.AuthServer
	authFailures      int
	streamProtocol    gortsplib.StreamProtocol
	streamTracks      map[int]*clientTrack
	rtcpReceivers     []*gortsplib.RtcpReceiver
	udpLastFrameTimes []*int64
	describeCSeq      gortsplib.HeaderValue
	describeUrl       string

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
		c.p.readersMap.remove(c)

	case clientStateRecord:
		atomic.AddInt64(&c.p.countPublisher, -1)

		if c.streamProtocol == gortsplib.StreamProtocolUDP {
			for _, track := range c.streamTracks {
				addr := makeUDPPublisherAddr(c.ip(), track.rtpPort)
				c.p.udpPublishersMap.remove(addr)

				addr = makeUDPPublisherAddr(c.ip(), track.rtcpPort)
				c.p.udpPublishersMap.remove(addr)
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
		var err error
		onConnectCmd, err = startExternalCommand(c.p.conf.RunOnConnect, "")
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

	// in RTSP, the control path is inserted after the query.
	// therefore, path and query can't be threated separately
	if req.Url.RawQuery != "" {
		pathName += "?" + req.Url.RawQuery
	}

	switch req.Method {
	case gortsplib.OPTIONS:
		c.conn.WriteResponse(&gortsplib.Response{
			StatusCode: gortsplib.StatusOK,
			Header: gortsplib.Header{
				"CSeq": cseq,
				"Public": gortsplib.HeaderValue{strings.Join([]string{
					string(gortsplib.GET_PARAMETER),
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

	// GET_PARAMETER is used like a ping
	case gortsplib.GET_PARAMETER:
		c.conn.WriteResponse(&gortsplib.Response{
			StatusCode: gortsplib.StatusOK,
			Header: gortsplib.Header{
				"CSeq":         cseq,
				"Content-Type": gortsplib.HeaderValue{"text/parameters"},
			},
			Content: []byte("\n"),
		})
		return nil

	case gortsplib.DESCRIBE:
		if c.state != clientStateInitial {
			c.writeResError(cseq, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, clientStateInitial))
			return errRunTerminate
		}

		pathName = removeQueryFromPath(pathName)

		pathConf, err := c.p.conf.checkPathNameAndFindConf(pathName)
		if err != nil {
			c.writeResError(cseq, gortsplib.StatusBadRequest, err)
			return errRunTerminate
		}

		err = c.authenticate(pathConf.readIpsParsed, pathConf.ReadUser, pathConf.ReadPass, req)
		if err != nil {
			if err == errAuthCritical {
				return errRunTerminate
			}
			return nil
		}

		c.p.clientDescribe <- clientDescribeReq{c, pathName, pathConf}

		c.describeCSeq = cseq
		c.describeUrl = req.Url.String()

		return errRunWaitDescription

	case gortsplib.ANNOUNCE:
		if c.state != clientStateInitial {
			c.writeResError(cseq, gortsplib.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, clientStateInitial))
			return errRunTerminate
		}

		pathName = removeQueryFromPath(pathName)

		pathConf, err := c.p.conf.checkPathNameAndFindConf(pathName)
		if err != nil {
			c.writeResError(cseq, gortsplib.StatusBadRequest, err)
			return errRunTerminate
		}

		err = c.authenticate(pathConf.publishIpsParsed, pathConf.PublishUser, pathConf.PublishPass, req)
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

		tracks, err := gortsplib.ReadTracks(req.Content)
		if err != nil {
			c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("invalid SDP: %s", err))
			return errRunTerminate
		}

		if len(tracks) == 0 {
			c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("no tracks defined"))
			return errRunTerminate
		}

		sdp := tracks.Write()

		res := make(chan error)
		c.p.clientAnnounce <- clientAnnounceReq{res, c, pathName, pathConf, len(tracks), sdp}
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

		if th.Cast != nil && *th.Cast == gortsplib.StreamMulticast {
			c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("multicast is not supported"))
			return errRunTerminate
		}

		basePath, controlPath, err := splitPath(pathName)
		if err != nil {
			c.writeResError(cseq, gortsplib.StatusBadRequest, err)
			return errRunTerminate
		}

		basePath = removeQueryFromPath(basePath)

		switch c.state {
		// play
		case clientStateInitial, clientStatePrePlay:
			pathConf, err := c.p.conf.checkPathNameAndFindConf(basePath)
			if err != nil {
				c.writeResError(cseq, gortsplib.StatusBadRequest, err)
				return errRunTerminate
			}

			err = c.authenticate(pathConf.readIpsParsed, pathConf.ReadUser, pathConf.ReadPass, req)
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
			if th.Protocol == gortsplib.StreamProtocolUDP {
				if _, ok := c.p.conf.protocolsParsed[gortsplib.StreamProtocolUDP]; !ok {
					c.writeResError(cseq, gortsplib.StatusUnsupportedTransport, fmt.Errorf("UDP streaming is disabled"))
					return errRunTerminate
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolUDP {
					c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("can't receive tracks with different protocols"))
					return errRunTerminate
				}

				if th.ClientPorts == nil {
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
					rtpPort:  (*th.ClientPorts)[0],
					rtcpPort: (*th.ClientPorts)[1],
				}

				th := &gortsplib.HeaderTransport{
					Protocol: gortsplib.StreamProtocolUDP,
					Cast: func() *gortsplib.StreamCast {
						v := gortsplib.StreamUnicast
						return &v
					}(),
					ClientPorts: th.ClientPorts,
					ServerPorts: &[2]int{c.p.conf.RtpPort, c.p.conf.RtcpPort},
				}

				c.conn.WriteResponse(&gortsplib.Response{
					StatusCode: gortsplib.StatusOK,
					Header: gortsplib.Header{
						"CSeq":      cseq,
						"Transport": th.Write(),
						"Session":   gortsplib.HeaderValue{"12345678"},
					},
				})
				return nil

				// play via TCP
			} else {
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

				interleavedIds := [2]int{trackId * 2, (trackId * 2) + 1}

				th := &gortsplib.HeaderTransport{
					Protocol:       gortsplib.StreamProtocolTCP,
					InterleavedIds: &interleavedIds,
				}

				c.conn.WriteResponse(&gortsplib.Response{
					StatusCode: gortsplib.StatusOK,
					Header: gortsplib.Header{
						"CSeq":      cseq,
						"Transport": th.Write(),
						"Session":   gortsplib.HeaderValue{"12345678"},
					},
				})
				return nil
			}

		// record
		case clientStatePreRecord:
			if th.Mode == nil || *th.Mode != "record" {
				c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not contain mode=record"))
				return errRunTerminate
			}

			// after ANNOUNCE, c.path is already set
			if basePath != c.path.name {
				c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.name, basePath))
				return errRunTerminate
			}

			// record via UDP
			if th.Protocol == gortsplib.StreamProtocolUDP {
				if _, ok := c.p.conf.protocolsParsed[gortsplib.StreamProtocolUDP]; !ok {
					c.writeResError(cseq, gortsplib.StatusUnsupportedTransport, fmt.Errorf("UDP streaming is disabled"))
					return errRunTerminate
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolUDP {
					c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("can't publish tracks with different protocols"))
					return errRunTerminate
				}

				if th.ClientPorts == nil {
					c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not have valid client ports (%s)", req.Header["Transport"]))
					return errRunTerminate
				}

				if len(c.streamTracks) >= c.path.publisherTrackCount {
					c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("all the tracks have already been setup"))
					return errRunTerminate
				}

				c.streamProtocol = gortsplib.StreamProtocolUDP
				c.streamTracks[len(c.streamTracks)] = &clientTrack{
					rtpPort:  (*th.ClientPorts)[0],
					rtcpPort: (*th.ClientPorts)[1],
				}

				th := &gortsplib.HeaderTransport{
					Protocol: gortsplib.StreamProtocolUDP,
					Cast: func() *gortsplib.StreamCast {
						v := gortsplib.StreamUnicast
						return &v
					}(),
					ClientPorts: th.ClientPorts,
					ServerPorts: &[2]int{c.p.conf.RtpPort, c.p.conf.RtcpPort},
				}

				c.conn.WriteResponse(&gortsplib.Response{
					StatusCode: gortsplib.StatusOK,
					Header: gortsplib.Header{
						"CSeq":      cseq,
						"Transport": th.Write(),
						"Session":   gortsplib.HeaderValue{"12345678"},
					},
				})
				return nil

				// record via TCP
			} else {
				if _, ok := c.p.conf.protocolsParsed[gortsplib.StreamProtocolTCP]; !ok {
					c.writeResError(cseq, gortsplib.StatusUnsupportedTransport, fmt.Errorf("TCP streaming is disabled"))
					return errRunTerminate
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolTCP {
					c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("can't publish tracks with different protocols"))
					return errRunTerminate
				}

				interleavedIds := [2]int{len(c.streamTracks) * 2, 1 + len(c.streamTracks)*2}

				if th.InterleavedIds == nil {
					c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("transport header does not contain the interleaved field"))
					return errRunTerminate
				}

				if (*th.InterleavedIds)[0] != interleavedIds[0] || (*th.InterleavedIds)[1] != interleavedIds[1] {
					c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("wrong interleaved ids, expected %v, got %v", interleavedIds, *th.InterleavedIds))
					return errRunTerminate
				}

				if len(c.streamTracks) >= c.path.publisherTrackCount {
					c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("all the tracks have already been setup"))
					return errRunTerminate
				}

				c.streamProtocol = gortsplib.StreamProtocolTCP
				c.streamTracks[len(c.streamTracks)] = &clientTrack{
					rtpPort:  0,
					rtcpPort: 0,
				}

				ht := &gortsplib.HeaderTransport{
					Protocol:       gortsplib.StreamProtocolTCP,
					InterleavedIds: &interleavedIds,
				}

				c.conn.WriteResponse(&gortsplib.Response{
					StatusCode: gortsplib.StatusOK,
					Header: gortsplib.Header{
						"CSeq":      cseq,
						"Transport": ht.Write(),
						"Session":   gortsplib.HeaderValue{"12345678"},
					},
				})
				return nil
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

		pathName = removeQueryFromPath(pathName)

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

		pathName = removeQueryFromPath(pathName)

		// path can end with a slash, remove it
		pathName = strings.TrimSuffix(pathName, "/")

		if pathName != c.path.name {
			c.writeResError(cseq, gortsplib.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.name, pathName))
			return errRunTerminate
		}

		if len(c.streamTracks) != c.path.publisherTrackCount {
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
	if c.path.conf.RunOnRead != "" {
		var err error
		onReadCmd, err = startExternalCommand(c.path.conf.RunOnRead, c.path.name)
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
	readRequest := make(chan readRequestPair)
	defer close(readRequest)

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
				res := make(chan error)
				readRequest <- readRequestPair{recvt, res}
				err := <-res
				if err != nil {
					readDone <- err
					break
				}
			}
		}
	}()

	for {
		select {
		// responses must be written in the same routine of frames
		case req := <-readRequest:
			req.res <- c.handleRequest(req.req)

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
			go func() {
				for req := range readRequest {
					req.res <- fmt.Errorf("terminated")
				}
			}()
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

	if c.streamProtocol == gortsplib.StreamProtocolUDP {
		c.udpLastFrameTimes = make([]*int64, len(c.streamTracks))
		for trackId := range c.streamTracks {
			v := time.Now().Unix()
			c.udpLastFrameTimes[trackId] = &v
		}
	}

	c.p.clientRecord <- c

	c.log("is publishing on path '%s', %d %s via %s", c.path.name, len(c.streamTracks), func() string {
		if len(c.streamTracks) == 1 {
			return "track"
		}
		return "tracks"
	}(), c.streamProtocol)

	var onPublishCmd *exec.Cmd
	if c.path.conf.RunOnPublish != "" {
		var err error
		onPublishCmd, err = startExternalCommand(c.path.conf.RunOnPublish, c.path.name)
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
			now := time.Now()

			for _, lastUnix := range c.udpLastFrameTimes {
				last := time.Unix(atomic.LoadInt64(lastUnix), 0)

				if now.Sub(last) >= c.p.conf.ReadTimeout {
					c.log("ERR: no packets received recently (maybe there's a firewall/NAT in between)")
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
	readBuf := newMultiBuffer(2, clientTCPReadBufferSize)

	readRequest := make(chan readRequestPair)
	defer close(readRequest)

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

				c.p.readersMap.forwardFrame(c.path, frame.TrackId, frame.StreamType, frame.Content)

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
		// responses must be written in the same routine of receiver reports
		case req := <-readRequest:
			req.res <- c.handleRequest(req.req)

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
			go func() {
				for req := range readRequest {
					req.res <- fmt.Errorf("terminated")
				}
			}()
			c.conn.Close()
			<-readDone
			return
		}
	}
}
