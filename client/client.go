package client

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/auth"
	"github.com/aler9/gortsplib/base"
	"github.com/aler9/gortsplib/headers"
	"github.com/aler9/gortsplib/rtcpreceiver"

	"github.com/aler9/rtsp-simple-server/conf"
	"github.com/aler9/rtsp-simple-server/externalcmd"
	"github.com/aler9/rtsp-simple-server/serverudp"
	"github.com/aler9/rtsp-simple-server/stats"
)

const (
	checkStreamInterval    = 5 * time.Second
	receiverReportInterval = 10 * time.Second
)

type readRequestPair struct {
	req *base.Request
	res chan error
}

type streamTrack struct {
	rtpPort  int
	rtcpPort int
}

type describeData struct {
	sdp      []byte
	redirect string
	err      error
}

type state int

const (
	stateInitial state = iota
	stateWaitingDescribe
	statePrePlay
	statePlay
	statePreRecord
	stateRecord
)

func (cs state) String() string {
	switch cs {
	case stateInitial:
		return "Initial"

	case stateWaitingDescribe:
		return "WaitingDescribe"

	case statePrePlay:
		return "PrePlay"

	case statePlay:
		return "Play"

	case statePreRecord:
		return "PreRecord"

	case stateRecord:
		return "Record"
	}
	return "Invalid"
}

type Path interface {
	Name() string
	SourceTrackCount() int
	Conf() *conf.PathConf
	OnClientRemove(*Client)
	OnClientPlay(*Client)
	OnClientRecord(*Client)
	OnFrame(int, gortsplib.StreamType, []byte)
}

type Parent interface {
	Log(string, ...interface{})
	OnClientClose(*Client)
	OnClientDescribe(*Client, string, *base.Request) (Path, error)
	OnClientAnnounce(*Client, string, gortsplib.Tracks, *base.Request) (Path, error)
	OnClientSetupPlay(*Client, string, int, *base.Request) (Path, error)
}

type Client struct {
	wg            *sync.WaitGroup
	stats         *stats.Stats
	serverUdpRtp  *serverudp.Server
	serverUdpRtcp *serverudp.Server
	readTimeout   time.Duration
	runOnConnect  string
	protocols     map[gortsplib.StreamProtocol]struct{}
	conn          *gortsplib.ConnServer
	parent        Parent

	state             state
	path              Path
	authUser          string
	authPass          string
	authHelper        *auth.Server
	authFailures      int
	streamProtocol    gortsplib.StreamProtocol
	streamTracks      map[int]*streamTrack
	rtcpReceivers     []*rtcpreceiver.RtcpReceiver
	udpLastFrameTimes []*int64
	describeCSeq      base.HeaderValue
	describeUrl       string

	// in
	describeData chan describeData           // from path
	tcpFrame     chan *base.InterleavedFrame // from source
	terminate    chan struct{}
}

func New(
	wg *sync.WaitGroup,
	stats *stats.Stats,
	serverUdpRtp *serverudp.Server,
	serverUdpRtcp *serverudp.Server,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	runOnConnect string,
	protocols map[gortsplib.StreamProtocol]struct{},
	nconn net.Conn,
	parent Parent) *Client {

	c := &Client{
		wg:            wg,
		stats:         stats,
		serverUdpRtp:  serverUdpRtp,
		serverUdpRtcp: serverUdpRtcp,
		readTimeout:   readTimeout,
		runOnConnect:  runOnConnect,
		protocols:     protocols,
		conn: gortsplib.NewConnServer(gortsplib.ConnServerConf{
			Conn:            nconn,
			ReadTimeout:     readTimeout,
			WriteTimeout:    writeTimeout,
			ReadBufferCount: 2,
		}),
		parent:       parent,
		state:        stateInitial,
		streamTracks: make(map[int]*streamTrack),
		describeData: make(chan describeData),
		tcpFrame:     make(chan *base.InterleavedFrame),
		terminate:    make(chan struct{}),
	}

	atomic.AddInt64(c.stats.CountClients, 1)
	c.log("connected")

	c.wg.Add(1)
	go c.run()
	return c
}

func (c *Client) Close() {
	atomic.AddInt64(c.stats.CountClients, -1)
	close(c.terminate)
}

func (c *Client) IsSource() {}

func (c *Client) log(format string, args ...interface{}) {
	c.parent.Log("[client %s] "+format, append([]interface{}{c.conn.NetConn().RemoteAddr().String()}, args...)...)
}

func (c *Client) ip() net.IP {
	return c.conn.NetConn().RemoteAddr().(*net.TCPAddr).IP
}

func (c *Client) zone() string {
	return c.conn.NetConn().RemoteAddr().(*net.TCPAddr).Zone
}

var errRunTerminate = errors.New("terminate")
var errRunWaitingDescribe = errors.New("wait description")
var errRunPlay = errors.New("play")
var errRunRecord = errors.New("record")

func (c *Client) run() {
	defer c.wg.Done()
	defer c.log("disconnected")

	var onConnectCmd *externalcmd.ExternalCmd
	if c.runOnConnect != "" {
		var err error
		onConnectCmd, err = externalcmd.New(c.runOnConnect, "")
		if err != nil {
			c.log("ERR: %s", err)
		}
	}

	for {
		if !c.runInitial() {
			break
		}
	}

	if c.path != nil {
		c.path.OnClientRemove(c)
		c.path = nil
	}

	if onConnectCmd != nil {
		onConnectCmd.Close()
	}

	close(c.describeData)
	close(c.tcpFrame)
}

func (c *Client) writeResError(cseq base.HeaderValue, code base.StatusCode, err error) {
	c.log("ERR: %s", err)

	c.conn.WriteResponse(&base.Response{
		StatusCode: code,
		Header: base.Header{
			"CSeq": cseq,
		},
	})
}

type ErrAuthNotCritical struct {
	*base.Response
}

func (ErrAuthNotCritical) Error() string {
	return "auth not critical"
}

type ErrAuthCritical struct {
	*base.Response
}

func (ErrAuthCritical) Error() string {
	return "auth critical"
}

func (c *Client) Authenticate(authMethods []headers.AuthMethod, ips []interface{}, user string, pass string, req *base.Request) error {
	// validate ip
	if ips != nil {
		ip := c.ip()

		if !ipEqualOrInRange(ip, ips) {
			c.log("ERR: ip '%s' not allowed", ip)

			return ErrAuthCritical{&base.Response{
				StatusCode: base.StatusUnauthorized,
				Header: base.Header{
					"CSeq":             req.Header["CSeq"],
					"WWW-Authenticate": c.authHelper.GenerateHeader(),
				},
			}}
		}
	}

	// validate user
	if user != "" {
		// reset authHelper every time the credentials change
		if c.authHelper == nil || c.authUser != user || c.authPass != pass {
			c.authUser = user
			c.authPass = pass
			c.authHelper = auth.NewServer(user, pass, authMethods)
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
			if c.authFailures > 3 {
				c.log("ERR: unauthorized: %s", err)

				return ErrAuthCritical{&base.Response{
					StatusCode: base.StatusUnauthorized,
					Header: base.Header{
						"CSeq":             req.Header["CSeq"],
						"WWW-Authenticate": c.authHelper.GenerateHeader(),
					},
				}}

			} else {
				if c.authFailures > 1 {
					c.log("WARN: unauthorized: %s", err)
				}

				return ErrAuthNotCritical{&base.Response{
					StatusCode: base.StatusUnauthorized,
					Header: base.Header{
						"CSeq":             req.Header["CSeq"],
						"WWW-Authenticate": c.authHelper.GenerateHeader(),
					},
				}}
			}
		}
	}

	// login successful, reset authFailures
	c.authFailures = 0

	return nil
}

func (c *Client) handleRequest(req *base.Request) error {
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
	// therefore, path and query can't be treated separately
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
		if c.state != stateInitial {
			c.writeResError(cseq, base.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, stateInitial))
			return errRunTerminate
		}

		pathName = removeQueryFromPath(pathName)

		path, err := c.parent.OnClientDescribe(c, pathName, req)
		if err != nil {
			switch terr := err.(type) {
			case ErrAuthNotCritical:
				c.conn.WriteResponse(terr.Response)
				return nil

			case ErrAuthCritical:
				c.conn.WriteResponse(terr.Response)
				return errRunTerminate

			default:
				c.writeResError(cseq, base.StatusBadRequest, err)
				return errRunTerminate
			}
		}

		c.path = path
		c.state = stateWaitingDescribe
		c.describeCSeq = cseq
		c.describeUrl = req.Url.String()

		return errRunWaitingDescribe

	case base.ANNOUNCE:
		if c.state != stateInitial {
			c.writeResError(cseq, base.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, stateInitial))
			return errRunTerminate
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

		pathName = removeQueryFromPath(pathName)

		path, err := c.parent.OnClientAnnounce(c, pathName, tracks, req)
		if err != nil {
			switch terr := err.(type) {
			case ErrAuthNotCritical:
				c.conn.WriteResponse(terr.Response)
				return nil

			case ErrAuthCritical:
				c.conn.WriteResponse(terr.Response)
				return errRunTerminate

			default:
				c.writeResError(cseq, base.StatusBadRequest, err)
				return errRunTerminate
			}
		}

		c.path = path
		c.state = statePreRecord

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

		basePath, controlPath, err := splitPathIntoBaseAndControl(pathName)
		if err != nil {
			c.writeResError(cseq, base.StatusBadRequest, err)
			return errRunTerminate
		}

		basePath = removeQueryFromPath(basePath)

		switch c.state {
		// play
		case stateInitial, statePrePlay:
			if th.Mode != nil && *th.Mode != gortsplib.TransportModePlay {
				c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("transport header must contain mode=play or not contain a mode"))
				return errRunTerminate
			}

			if c.path != nil && basePath != c.path.Name() {
				c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.Name(), basePath))
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
				if _, ok := c.protocols[gortsplib.StreamProtocolUDP]; !ok {
					c.writeResError(cseq, base.StatusUnsupportedTransport, fmt.Errorf("UDP streaming is disabled"))
					return nil
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolUDP {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("can't receive tracks with different protocols"))
					return errRunTerminate
				}

				if th.ClientPorts == nil {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("transport header does not have valid client ports (%v)", req.Header["Transport"]))
					return errRunTerminate
				}

				path, err := c.parent.OnClientSetupPlay(c, basePath, trackId, req)
				if err != nil {
					switch terr := err.(type) {
					case ErrAuthNotCritical:
						c.conn.WriteResponse(terr.Response)
						return nil

					case ErrAuthCritical:
						c.conn.WriteResponse(terr.Response)
						return errRunTerminate

					default:
						c.writeResError(cseq, base.StatusBadRequest, err)
						return errRunTerminate
					}
				}

				c.path = path
				c.state = statePrePlay

				c.streamProtocol = gortsplib.StreamProtocolUDP
				c.streamTracks[trackId] = &streamTrack{
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
					ServerPorts: &[2]int{c.serverUdpRtp.Port(), c.serverUdpRtcp.Port()},
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
				if _, ok := c.protocols[gortsplib.StreamProtocolTCP]; !ok {
					c.writeResError(cseq, base.StatusUnsupportedTransport, fmt.Errorf("TCP streaming is disabled"))
					return nil
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolTCP {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("can't receive tracks with different protocols"))
					return errRunTerminate
				}

				path, err := c.parent.OnClientSetupPlay(c, basePath, trackId, req)
				if err != nil {
					switch terr := err.(type) {
					case ErrAuthNotCritical:
						c.conn.WriteResponse(terr.Response)
						return nil

					case ErrAuthCritical:
						c.conn.WriteResponse(terr.Response)
						return errRunTerminate

					default:
						c.writeResError(cseq, base.StatusBadRequest, err)
						return errRunTerminate
					}
				}

				c.path = path
				c.state = statePrePlay

				c.streamProtocol = gortsplib.StreamProtocolTCP
				c.streamTracks[trackId] = &streamTrack{
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
		case statePreRecord:
			if th.Mode == nil || *th.Mode != gortsplib.TransportModeRecord {
				c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("transport header does not contain mode=record"))
				return errRunTerminate
			}

			// after ANNOUNCE, c.path is already set
			if basePath != c.path.Name() {
				c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.Name(), basePath))
				return errRunTerminate
			}

			// record with UDP
			if th.Protocol == gortsplib.StreamProtocolUDP {
				if _, ok := c.protocols[gortsplib.StreamProtocolUDP]; !ok {
					c.writeResError(cseq, base.StatusUnsupportedTransport, fmt.Errorf("UDP streaming is disabled"))
					return nil
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolUDP {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("can't publish tracks with different protocols"))
					return errRunTerminate
				}

				if th.ClientPorts == nil {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("transport header does not have valid client ports (%s)", req.Header["Transport"]))
					return errRunTerminate
				}

				if len(c.streamTracks) >= c.path.SourceTrackCount() {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("all the tracks have already been setup"))
					return errRunTerminate
				}

				c.streamProtocol = gortsplib.StreamProtocolUDP
				c.streamTracks[len(c.streamTracks)] = &streamTrack{
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
					ServerPorts: &[2]int{c.serverUdpRtp.Port(), c.serverUdpRtcp.Port()},
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
				if _, ok := c.protocols[gortsplib.StreamProtocolTCP]; !ok {
					c.writeResError(cseq, base.StatusUnsupportedTransport, fmt.Errorf("TCP streaming is disabled"))
					return nil
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

				if len(c.streamTracks) >= c.path.SourceTrackCount() {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("all the tracks have already been setup"))
					return errRunTerminate
				}

				c.streamProtocol = gortsplib.StreamProtocolTCP
				c.streamTracks[len(c.streamTracks)] = &streamTrack{
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
		if c.state != statePrePlay {
			c.writeResError(cseq, base.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, statePrePlay))
			return errRunTerminate
		}

		pathName = removeQueryFromPath(pathName)

		// path can end with a slash, remove it
		pathName = strings.TrimSuffix(pathName, "/")

		if pathName != c.path.Name() {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.Name(), pathName))
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
		if c.state != statePreRecord {
			c.writeResError(cseq, base.StatusBadRequest,
				fmt.Errorf("client is in state '%s' instead of '%s'", c.state, statePreRecord))
			return errRunTerminate
		}

		pathName = removeQueryFromPath(pathName)

		// path can end with a slash, remove it
		pathName = strings.TrimSuffix(pathName, "/")

		if pathName != c.path.Name() {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.Name(), pathName))
			return errRunTerminate
		}

		if len(c.streamTracks) != c.path.SourceTrackCount() {
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

func (c *Client) runInitial() bool {
	readDone := make(chan error)
	go func() {
		for {
			req, err := c.conn.ReadRequest()
			if err != nil {
				readDone <- err
				return
			}

			err = c.handleRequest(req)
			if err != nil {
				readDone <- err
				return
			}
		}
	}()

	select {
	case err := <-readDone:
		switch err {
		case errRunWaitingDescribe:
			return c.runWaitingDescribe()

		case errRunPlay:
			return c.runPlay()

		case errRunRecord:
			return c.runRecord()

		default:
			c.conn.Close()
			if err != io.EOF && err != errRunTerminate {
				c.log("ERR: %s", err)
			}

			c.parent.OnClientClose(c)
			<-c.terminate
			return false
		}

	case <-c.terminate:
		c.conn.Close()
		<-readDone
		return false
	}
}

func (c *Client) runWaitingDescribe() bool {
	select {
	case res := <-c.describeData:
		c.path.OnClientRemove(c)
		c.path = nil

		c.state = stateInitial

		if res.err != nil {
			c.writeResError(c.describeCSeq, base.StatusNotFound, res.err)
			return true
		}

		if res.redirect != "" {
			c.conn.WriteResponse(&base.Response{
				StatusCode: base.StatusMovedPermanently,
				Header: base.Header{
					"CSeq":     c.describeCSeq,
					"Location": base.HeaderValue{res.redirect},
				},
			})
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
		go func() {
			for range c.describeData {
			}
		}()

		c.conn.Close()
		return false
	}
}

func (c *Client) runPlay() bool {
	// start sending frames only after replying to the PLAY request
	c.path.OnClientPlay(c)

	c.log("is reading from path '%s', %d %s with %s", c.path.Name(), len(c.streamTracks), func() string {
		if len(c.streamTracks) == 1 {
			return "track"
		}
		return "tracks"
	}(), c.streamProtocol)

	var onReadCmd *externalcmd.ExternalCmd
	if c.path.Conf().RunOnRead != "" {
		var err error
		onReadCmd, err = externalcmd.New(c.path.Conf().RunOnRead, c.path.Name())
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
		onReadCmd.Close()
	}

	return false
}

func (c *Client) runPlayUDP() {
	readDone := make(chan error)
	go func() {
		for {
			req, err := c.conn.ReadRequest()
			if err != nil {
				readDone <- err
				return
			}

			err = c.handleRequest(req)
			if err != nil {
				readDone <- err
				return
			}
		}
	}()

	select {
	case err := <-readDone:
		c.conn.Close()
		if err != io.EOF && err != errRunTerminate {
			c.log("ERR: %s", err)
		}

		c.parent.OnClientClose(c)
		<-c.terminate
		return

	case <-c.terminate:
		c.conn.Close()
		<-readDone
		return
	}
}

func (c *Client) runPlayTCP() {
	readRequest := make(chan readRequestPair)
	defer close(readRequest)

	readDone := make(chan error)
	go func() {
		for {
			recv, err := c.conn.ReadFrameTCPOrRequest(false)
			if err != nil {
				readDone <- err
				return
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
					return
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

			c.parent.OnClientClose(c)
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

			go func() {
				for range c.tcpFrame {
				}
			}()

			c.conn.Close()
			<-readDone
			return
		}
	}
}

func (c *Client) runRecord() bool {
	c.path.OnClientRecord(c)

	c.log("is publishing to path '%s', %d %s with %s", c.path.Name(), len(c.streamTracks), func() string {
		if len(c.streamTracks) == 1 {
			return "track"
		}
		return "tracks"
	}(), c.streamProtocol)

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

		for trackId, track := range c.streamTracks {
			c.serverUdpRtp.AddPublisher(c.ip(), track.rtpPort, c, trackId)
			c.serverUdpRtcp.AddPublisher(c.ip(), track.rtcpPort, c, trackId)
		}

		// open the firewall by sending packets to the counterpart
		for _, track := range c.streamTracks {
			c.serverUdpRtp.Write(
				[]byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
				&net.UDPAddr{
					IP:   c.ip(),
					Zone: c.zone(),
					Port: track.rtpPort,
				})

			c.serverUdpRtcp.Write(
				[]byte{0x80, 0xc9, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00},
				&net.UDPAddr{
					IP:   c.ip(),
					Zone: c.zone(),
					Port: track.rtcpPort,
				})
		}
	}

	var onPublishCmd *externalcmd.ExternalCmd
	if c.path.Conf().RunOnPublish != "" {
		var err error
		onPublishCmd, err = externalcmd.New(c.path.Conf().RunOnPublish, c.path.Name())
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
		onPublishCmd.Close()
	}

	if c.streamProtocol == gortsplib.StreamProtocolUDP {
		for _, track := range c.streamTracks {
			c.serverUdpRtp.RemovePublisher(c.ip(), track.rtpPort, c)
			c.serverUdpRtcp.RemovePublisher(c.ip(), track.rtcpPort, c)
		}
	}

	return false
}

func (c *Client) runRecordUDP() {
	readDone := make(chan error)
	go func() {
		for {
			req, err := c.conn.ReadRequest()
			if err != nil {
				readDone <- err
				return
			}

			err = c.handleRequest(req)
			if err != nil {
				readDone <- err
				return
			}
		}
	}()

	checkStreamTicker := time.NewTicker(checkStreamInterval)
	defer checkStreamTicker.Stop()

	receiverReportTicker := time.NewTicker(receiverReportInterval)
	defer receiverReportTicker.Stop()

	for {
		select {
		case err := <-readDone:
			c.conn.Close()
			if err != io.EOF && err != errRunTerminate {
				c.log("ERR: %s", err)
			}

			c.parent.OnClientClose(c)
			<-c.terminate
			return

		case <-checkStreamTicker.C:
			now := time.Now()

			for _, lastUnix := range c.udpLastFrameTimes {
				last := time.Unix(atomic.LoadInt64(lastUnix), 0)

				if now.Sub(last) >= c.readTimeout {
					c.log("ERR: no packets received recently (maybe there's a firewall/NAT in between)")
					c.conn.Close()
					<-readDone

					c.parent.OnClientClose(c)
					<-c.terminate
					return
				}
			}

		case <-receiverReportTicker.C:
			for trackId := range c.streamTracks {
				frame := c.rtcpReceivers[trackId].Report()
				c.serverUdpRtcp.Write(frame, &net.UDPAddr{
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

func (c *Client) runRecordTCP() {
	readRequest := make(chan readRequestPair)
	defer close(readRequest)

	readDone := make(chan error)
	go func() {
		for {
			recv, err := c.conn.ReadFrameTCPOrRequest(true)
			if err != nil {
				readDone <- err
				return
			}

			switch recvt := recv.(type) {
			case *base.InterleavedFrame:
				if recvt.TrackId >= len(c.streamTracks) {
					readDone <- fmt.Errorf("invalid track id '%d'", recvt.TrackId)
					return
				}

				c.rtcpReceivers[recvt.TrackId].OnFrame(recvt.StreamType, recvt.Content)
				c.path.OnFrame(recvt.TrackId, recvt.StreamType, recvt.Content)

			case *base.Request:
				err := c.handleRequest(recvt)
				if err != nil {
					readDone <- err
					return
				}
			}
		}
	}()

	receiverReportTicker := time.NewTicker(receiverReportInterval)
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

			c.parent.OnClientClose(c)
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

func (c *Client) OnUdpPublisherFrame(trackId int, streamType base.StreamType, buf []byte) {
	atomic.StoreInt64(c.udpLastFrameTimes[trackId], time.Now().Unix())

	c.rtcpReceivers[trackId].OnFrame(streamType, buf)
	c.path.OnFrame(trackId, streamType, buf)
}

func (c *Client) OnReaderFrame(trackId int, streamType base.StreamType, buf []byte) {
	track, ok := c.streamTracks[trackId]
	if !ok {
		return
	}

	if c.streamProtocol == gortsplib.StreamProtocolUDP {
		if streamType == gortsplib.StreamTypeRtp {
			c.serverUdpRtp.Write(buf, &net.UDPAddr{
				IP:   c.ip(),
				Zone: c.zone(),
				Port: track.rtpPort,
			})

		} else {
			c.serverUdpRtcp.Write(buf, &net.UDPAddr{
				IP:   c.ip(),
				Zone: c.zone(),
				Port: track.rtcpPort,
			})
		}

	} else {
		c.tcpFrame <- &base.InterleavedFrame{
			TrackId:    trackId,
			StreamType: streamType,
			Content:    buf,
		}
	}
}

func (c *Client) OnPathDescribeData(sdp []byte, redirect string, err error) {
	c.describeData <- describeData{sdp, redirect, err}
}
