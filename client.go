package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/auth"
	"github.com/aler9/gortsplib/base"
	"github.com/aler9/gortsplib/headers"
	"github.com/aler9/gortsplib/rtcpreceiver"
)

const (
	clientCheckStreamInterval    = 5 * time.Second
	clientReceiverReportInterval = 10 * time.Second
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
	req *base.Request
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
	authHelper        *auth.Server
	authFailures      int
	streamProtocol    gortsplib.StreamProtocol
	streamTracks      map[int]*clientTrack
	rtcpReceivers     []*rtcpreceiver.RtcpReceiver
	udpLastFrameTimes []*int64
	describeCSeq      base.HeaderValue
	describeUrl       string

	describe  chan describeRes
	tcpFrame  chan *base.InterleavedFrame
	terminate chan struct{}
	done      chan struct{}
}

func newClient(p *program, nconn net.Conn) *client {
	c := &client{
		p: p,
		conn: gortsplib.NewConnServer(gortsplib.ConnServerConf{
			Conn:            nconn,
			ReadTimeout:     p.conf.ReadTimeout,
			WriteTimeout:    p.conf.WriteTimeout,
			ReadBufferCount: 2,
		}),
		state:        clientStateInitial,
		streamTracks: make(map[int]*clientTrack),
		describe:     make(chan describeRes),
		tcpFrame:     make(chan *base.InterleavedFrame),
		terminate:    make(chan struct{}),
		done:         make(chan struct{}),
	}

	go c.run()
	return c
}

func (c *client) close() {
	delete(c.p.clients, c)

	atomic.AddInt64(c.p.countClients, -1)

	switch c.state {
	case clientStatePlay:
		atomic.AddInt64(c.p.countReaders, -1)
		c.p.readersMap.remove(c)

	case clientStateRecord:
		atomic.AddInt64(c.p.countPublishers, -1)

		if c.streamProtocol == gortsplib.StreamProtocolUDP {
			for _, track := range c.streamTracks {
				addr := makeUDPPublisherAddr(c.ip(), track.rtpPort)
				c.p.udpPublishersMap.remove(addr)

				addr = makeUDPPublisherAddr(c.ip(), track.rtcpPort)
				c.p.udpPublishersMap.remove(addr)
			}
		}

		c.path.onSourceSetNotReady()
	}

	if c.path != nil && c.path.source == c {
		c.path.onSourceRemove()
	}

	close(c.terminate)

	c.log("disconnected")
}

func (c *client) log(format string, args ...interface{}) {
	c.p.log("[client %s] "+format, append([]interface{}{c.conn.NetConn().RemoteAddr().String()}, args...)...)
}

func (c *client) isSource() {}

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
	var onConnectCmd *externalCmd
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
		onConnectCmd.close()
	}

	close(c.describe)
	close(c.tcpFrame)
	close(c.done)
}

func (c *client) writeResError(cseq base.HeaderValue, code base.StatusCode, err error) {
	c.log("ERR: %s", err)

	c.conn.WriteResponse(&base.Response{
		StatusCode: code,
		Header: base.Header{
			"CSeq": cseq,
		},
	})
}

var errAuthCritical = errors.New("auth critical")
var errAuthNotCritical = errors.New("auth not critical")

func (c *client) authenticate(ips []interface{}, user string, pass string, req *base.Request) error {
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
			c.authHelper = auth.NewServer(user, pass, c.p.conf.authMethodsParsed)
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

			c.conn.WriteResponse(&base.Response{
				StatusCode: base.StatusUnauthorized,
				Header: base.Header{
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

func (c *client) handleRequest(req *base.Request) error {
	c.log(string(req.Method))

	cseq, ok := req.Header["CSeq"]
	if !ok || len(cseq) != 1 {
		c.writeResError(nil, base.StatusBadRequest, fmt.Errorf("cseq missing"))
		return errRunTerminate
	}

	pathName := req.Url.Path
	if len(pathName) < 1 || pathName[0] != '/' {
		c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("path must begin with a slash"))
		return errRunTerminate
	}
	pathName = pathName[1:] // strip leading slash

	// in RTSP, the control path is inserted after the query.
	// therefore, path and query can't be threated separately
	if req.Url.RawQuery != "" {
		pathName += "?" + req.Url.RawQuery
	}

	switch req.Method {
	case base.OPTIONS:
		c.conn.WriteResponse(&base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"CSeq": cseq,
				"Public": base.HeaderValue{strings.Join([]string{
					string(base.GET_PARAMETER),
					string(base.DESCRIBE),
					string(base.ANNOUNCE),
					string(base.SETUP),
					string(base.PLAY),
					string(base.RECORD),
					string(base.TEARDOWN),
				}, ", ")},
			},
		})
		return nil

		// GET_PARAMETER is used like a ping
	case base.GET_PARAMETER:
		c.conn.WriteResponse(&base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"CSeq":         cseq,
				"Content-Type": base.HeaderValue{"text/parameters"},
			},
			Content: []byte("\n"),
		})
		return nil

	case base.DESCRIBE:
		if c.state != clientStateInitial {
			c.writeResError(cseq, base.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, clientStateInitial))
			return errRunTerminate
		}

		pathName = removeQueryFromPath(pathName)

		pathConf, err := c.p.conf.checkPathNameAndFindConf(pathName)
		if err != nil {
			c.writeResError(cseq, base.StatusBadRequest, err)
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

	case base.ANNOUNCE:
		if c.state != clientStateInitial {
			c.writeResError(cseq, base.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, clientStateInitial))
			return errRunTerminate
		}

		pathName = removeQueryFromPath(pathName)

		pathConf, err := c.p.conf.checkPathNameAndFindConf(pathName)
		if err != nil {
			c.writeResError(cseq, base.StatusBadRequest, err)
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
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("Content-Type header missing"))
			return errRunTerminate
		}

		if ct[0] != "application/sdp" {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("unsupported Content-Type '%s'", ct))
			return errRunTerminate
		}

		tracks, err := gortsplib.ReadTracks(req.Content)
		if err != nil {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("invalid SDP: %s", err))
			return errRunTerminate
		}

		if len(tracks) == 0 {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("no tracks defined"))
			return errRunTerminate
		}

		sdp := tracks.Write()

		res := make(chan error)
		c.p.clientAnnounce <- clientAnnounceReq{res, c, pathName, pathConf, len(tracks), sdp}
		err = <-res
		if err != nil {
			c.writeResError(cseq, base.StatusBadRequest, err)
			return errRunTerminate
		}

		c.conn.WriteResponse(&base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"CSeq": cseq,
			},
		})
		return nil

	case base.SETUP:
		th, err := headers.ReadTransport(req.Header["Transport"])
		if err != nil {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("transport header: %s", err))
			return errRunTerminate
		}

		if th.Cast != nil && *th.Cast == gortsplib.StreamMulticast {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("multicast is not supported"))
			return errRunTerminate
		}

		basePath, controlPath, err := splitPath(pathName)
		if err != nil {
			c.writeResError(cseq, base.StatusBadRequest, err)
			return errRunTerminate
		}

		basePath = removeQueryFromPath(basePath)

		switch c.state {
		// play
		case clientStateInitial, clientStatePrePlay:
			if th.Mode != nil && *th.Mode != gortsplib.TransportModePlay {
				c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("transport header must contain mode=play or not contain a mode"))
				return errRunTerminate
			}

			pathConf, err := c.p.conf.checkPathNameAndFindConf(basePath)
			if err != nil {
				c.writeResError(cseq, base.StatusBadRequest, err)
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
				c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.name, basePath))
				return errRunTerminate
			}

			if !strings.HasPrefix(controlPath, "trackID=") {
				c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("invalid control path (%s)", controlPath))
				return errRunTerminate
			}

			tmp, err := strconv.ParseInt(controlPath[len("trackID="):], 10, 64)
			if err != nil || tmp < 0 {
				c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("invalid track id (%s)", controlPath))
				return errRunTerminate
			}
			trackId := int(tmp)

			if _, ok := c.streamTracks[trackId]; ok {
				c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("track %d has already been setup", trackId))
				return errRunTerminate
			}

			// play with UDP
			if th.Protocol == gortsplib.StreamProtocolUDP {
				if _, ok := c.p.conf.protocolsParsed[gortsplib.StreamProtocolUDP]; !ok {
					c.writeResError(cseq, base.StatusUnsupportedTransport, fmt.Errorf("UDP streaming is disabled"))
					return errRunTerminate
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolUDP {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("can't receive tracks with different protocols"))
					return errRunTerminate
				}

				if th.ClientPorts == nil {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("transport header does not have valid client ports (%v)", req.Header["Transport"]))
					return errRunTerminate
				}

				res := make(chan error)
				c.p.clientSetupPlay <- clientSetupPlayReq{res, c, basePath, trackId}
				err = <-res
				if err != nil {
					c.writeResError(cseq, base.StatusBadRequest, err)
					return errRunTerminate
				}

				c.streamProtocol = gortsplib.StreamProtocolUDP
				c.streamTracks[trackId] = &clientTrack{
					rtpPort:  (*th.ClientPorts)[0],
					rtcpPort: (*th.ClientPorts)[1],
				}

				th := &headers.Transport{
					Protocol: gortsplib.StreamProtocolUDP,
					Cast: func() *gortsplib.StreamCast {
						v := gortsplib.StreamUnicast
						return &v
					}(),
					ClientPorts: th.ClientPorts,
					ServerPorts: &[2]int{c.p.conf.RtpPort, c.p.conf.RtcpPort},
				}

				c.conn.WriteResponse(&base.Response{
					StatusCode: base.StatusOK,
					Header: base.Header{
						"CSeq":      cseq,
						"Transport": th.Write(),
						"Session":   base.HeaderValue{"12345678"},
					},
				})
				return nil

				// play with TCP
			} else {
				if _, ok := c.p.conf.protocolsParsed[gortsplib.StreamProtocolTCP]; !ok {
					c.writeResError(cseq, base.StatusUnsupportedTransport, fmt.Errorf("TCP streaming is disabled"))
					return errRunTerminate
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolTCP {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("can't receive tracks with different protocols"))
					return errRunTerminate
				}

				res := make(chan error)
				c.p.clientSetupPlay <- clientSetupPlayReq{res, c, basePath, trackId}
				err = <-res
				if err != nil {
					c.writeResError(cseq, base.StatusBadRequest, err)
					return errRunTerminate
				}

				c.streamProtocol = gortsplib.StreamProtocolTCP
				c.streamTracks[trackId] = &clientTrack{
					rtpPort:  0,
					rtcpPort: 0,
				}

				interleavedIds := [2]int{trackId * 2, (trackId * 2) + 1}

				th := &headers.Transport{
					Protocol:       gortsplib.StreamProtocolTCP,
					InterleavedIds: &interleavedIds,
				}

				c.conn.WriteResponse(&base.Response{
					StatusCode: base.StatusOK,
					Header: base.Header{
						"CSeq":      cseq,
						"Transport": th.Write(),
						"Session":   base.HeaderValue{"12345678"},
					},
				})
				return nil
			}

		// record
		case clientStatePreRecord:
			if th.Mode == nil || *th.Mode != gortsplib.TransportModeRecord {
				c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("transport header does not contain mode=record"))
				return errRunTerminate
			}

			// after ANNOUNCE, c.path is already set
			if basePath != c.path.name {
				c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.name, basePath))
				return errRunTerminate
			}

			// record with UDP
			if th.Protocol == gortsplib.StreamProtocolUDP {
				if _, ok := c.p.conf.protocolsParsed[gortsplib.StreamProtocolUDP]; !ok {
					c.writeResError(cseq, base.StatusUnsupportedTransport, fmt.Errorf("UDP streaming is disabled"))
					return errRunTerminate
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolUDP {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("can't publish tracks with different protocols"))
					return errRunTerminate
				}

				if th.ClientPorts == nil {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("transport header does not have valid client ports (%s)", req.Header["Transport"]))
					return errRunTerminate
				}

				if len(c.streamTracks) >= c.path.sourceTrackCount {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("all the tracks have already been setup"))
					return errRunTerminate
				}

				c.streamProtocol = gortsplib.StreamProtocolUDP
				c.streamTracks[len(c.streamTracks)] = &clientTrack{
					rtpPort:  (*th.ClientPorts)[0],
					rtcpPort: (*th.ClientPorts)[1],
				}

				th := &headers.Transport{
					Protocol: gortsplib.StreamProtocolUDP,
					Cast: func() *gortsplib.StreamCast {
						v := gortsplib.StreamUnicast
						return &v
					}(),
					ClientPorts: th.ClientPorts,
					ServerPorts: &[2]int{c.p.conf.RtpPort, c.p.conf.RtcpPort},
				}

				c.conn.WriteResponse(&base.Response{
					StatusCode: base.StatusOK,
					Header: base.Header{
						"CSeq":      cseq,
						"Transport": th.Write(),
						"Session":   base.HeaderValue{"12345678"},
					},
				})
				return nil

				// record with TCP
			} else {
				if _, ok := c.p.conf.protocolsParsed[gortsplib.StreamProtocolTCP]; !ok {
					c.writeResError(cseq, base.StatusUnsupportedTransport, fmt.Errorf("TCP streaming is disabled"))
					return errRunTerminate
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolTCP {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("can't publish tracks with different protocols"))
					return errRunTerminate
				}

				interleavedIds := [2]int{len(c.streamTracks) * 2, 1 + len(c.streamTracks)*2}

				if th.InterleavedIds == nil {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("transport header does not contain the interleaved field"))
					return errRunTerminate
				}

				if (*th.InterleavedIds)[0] != interleavedIds[0] || (*th.InterleavedIds)[1] != interleavedIds[1] {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("wrong interleaved ids, expected %v, got %v", interleavedIds, *th.InterleavedIds))
					return errRunTerminate
				}

				if len(c.streamTracks) >= c.path.sourceTrackCount {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("all the tracks have already been setup"))
					return errRunTerminate
				}

				c.streamProtocol = gortsplib.StreamProtocolTCP
				c.streamTracks[len(c.streamTracks)] = &clientTrack{
					rtpPort:  0,
					rtcpPort: 0,
				}

				ht := &headers.Transport{
					Protocol:       gortsplib.StreamProtocolTCP,
					InterleavedIds: &interleavedIds,
				}

				c.conn.WriteResponse(&base.Response{
					StatusCode: base.StatusOK,
					Header: base.Header{
						"CSeq":      cseq,
						"Transport": ht.Write(),
						"Session":   base.HeaderValue{"12345678"},
					},
				})
				return nil
			}

		default:
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("client is in state '%s'", c.state))
			return errRunTerminate
		}

	case base.PLAY:
		if c.state != clientStatePrePlay {
			c.writeResError(cseq, base.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, clientStatePrePlay))
			return errRunTerminate
		}

		pathName = removeQueryFromPath(pathName)

		// path can end with a slash, remove it
		pathName = strings.TrimSuffix(pathName, "/")

		if pathName != c.path.name {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.name, pathName))
			return errRunTerminate
		}

		if len(c.streamTracks) == 0 {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("no tracks have been setup"))
			return errRunTerminate
		}

		// write response before setting state
		// otherwise, in case of TCP connections, RTP packets could be sent
		// before the response
		c.conn.WriteResponse(&base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"CSeq":    cseq,
				"Session": base.HeaderValue{"12345678"},
			},
		})

		return errRunPlay

	case base.RECORD:
		if c.state != clientStatePreRecord {
			c.writeResError(cseq, base.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, clientStatePreRecord))
			return errRunTerminate
		}

		pathName = removeQueryFromPath(pathName)

		// path can end with a slash, remove it
		pathName = strings.TrimSuffix(pathName, "/")

		if pathName != c.path.name {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.name, pathName))
			return errRunTerminate
		}

		if len(c.streamTracks) != c.path.sourceTrackCount {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("not all tracks have been setup"))
			return errRunTerminate
		}

		c.conn.WriteResponse(&base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"CSeq":    cseq,
				"Session": base.HeaderValue{"12345678"},
			},
		})

		return errRunRecord

	case base.TEARDOWN:
		// close connection silently
		return errRunTerminate

	default:
		c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("unhandled method '%s'", req.Method))
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
			c.writeResError(c.describeCSeq, base.StatusNotFound, res.err)
			return true
		}

		c.conn.WriteResponse(&base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"CSeq":         c.describeCSeq,
				"Content-Base": base.HeaderValue{c.describeUrl + "/"},
				"Content-Type": base.HeaderValue{"application/sdp"},
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

	c.log("is reading from path '%s', %d %s with %s", c.path.name, len(c.streamTracks), func() string {
		if len(c.streamTracks) == 1 {
			return "track"
		}
		return "tracks"
	}(), c.streamProtocol)

	var onReadCmd *externalCmd
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
		onReadCmd.close()
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
		for {
			recv, err := c.conn.ReadFrameTCPOrRequest(false)
			if err != nil {
				readDone <- err
				break
			}

			switch recvt := recv.(type) {
			case *base.InterleavedFrame:
				// rtcp feedback is handled by gortsplib

			case *base.Request:
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
			c.conn.WriteFrameTCP(frame.TrackId, frame.StreamType, frame.Content)

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
	c.rtcpReceivers = make([]*rtcpreceiver.RtcpReceiver, len(c.streamTracks))
	for trackId := range c.streamTracks {
		c.rtcpReceivers[trackId] = rtcpreceiver.New()
	}

	if c.streamProtocol == gortsplib.StreamProtocolUDP {
		c.udpLastFrameTimes = make([]*int64, len(c.streamTracks))
		for trackId := range c.streamTracks {
			v := time.Now().Unix()
			c.udpLastFrameTimes[trackId] = &v
		}
	}

	c.p.clientRecord <- c

	c.log("is publishing to path '%s', %d %s with %s", c.path.name, len(c.streamTracks), func() string {
		if len(c.streamTracks) == 1 {
			return "track"
		}
		return "tracks"
	}(), c.streamProtocol)

	var onPublishCmd *externalCmd
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
		onPublishCmd.close()
	}

	return false
}

func (c *client) runRecordUDP() {
	// open the firewall by sending packets to the counterpart
	for _, track := range c.streamTracks {
		c.p.serverUdpRtp.write(
			[]byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			&net.UDPAddr{
				IP:   c.ip(),
				Zone: c.zone(),
				Port: track.rtpPort,
			})

		c.p.serverUdpRtcp.write(
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
				c.p.serverUdpRtcp.write(frame, &net.UDPAddr{
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
	readRequest := make(chan readRequestPair)
	defer close(readRequest)

	readDone := make(chan error)
	go func() {
		for {
			recv, err := c.conn.ReadFrameTCPOrRequest(true)
			if err != nil {
				readDone <- err
				break
			}

			switch recvt := recv.(type) {
			case *base.InterleavedFrame:
				if recvt.TrackId >= len(c.streamTracks) {
					readDone <- fmt.Errorf("invalid track id '%d'", recvt.TrackId)
					break
				}

				c.rtcpReceivers[recvt.TrackId].OnFrame(recvt.StreamType, recvt.Content)

				c.p.readersMap.forwardFrame(c.path, recvt.TrackId, recvt.StreamType, recvt.Content)

			case *base.Request:
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
				c.conn.WriteFrameTCP(trackId, gortsplib.StreamTypeRtcp, frame)
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
