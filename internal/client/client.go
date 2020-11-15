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
	"github.com/aler9/gortsplib/pkg/auth"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/headers"
	"github.com/aler9/gortsplib/pkg/rtcpreceiver"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
	"github.com/aler9/rtsp-simple-server/internal/serverudp"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

const (
	checkStreamInterval    = 5 * time.Second
	receiverReportInterval = 10 * time.Second
	sessionId              = "12345678"
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

func (s state) String() string {
	switch s {
	case stateInitial:
		return "initial"
	case stateWaitingDescribe:
		return "waitingDescribe"
	case statePrePlay:
		return "prePlay"
	case statePlay:
		return "play"
	case statePreRecord:
		return "preRecord"
	case stateRecord:
		return "record"
	}
	return "invalid"
}

// Path is implemented by path.Path.
type Path interface {
	Name() string
	SourceTrackCount() int
	Conf() *conf.PathConf
	OnClientRemove(*Client)
	OnClientPlay(*Client)
	OnClientRecord(*Client)
	OnClientPause(*Client)
	OnFrame(int, gortsplib.StreamType, []byte)
}

// Parent is implemented by clientman.ClientMan.
type Parent interface {
	Log(string, ...interface{})
	OnClientClose(*Client)
	OnClientDescribe(*Client, string, *base.Request) (Path, error)
	OnClientAnnounce(*Client, string, gortsplib.Tracks, *base.Request) (Path, error)
	OnClientSetupPlay(*Client, string, int, *base.Request) (Path, error)
}

// Client is a RTSP client.
type Client struct {
	rtspPort            int
	readTimeout         time.Duration
	runOnConnect        string
	runOnConnectRestart bool
	protocols           map[gortsplib.StreamProtocol]struct{}
	wg                  *sync.WaitGroup
	stats               *stats.Stats
	serverUdpRtp        *serverudp.Server
	serverUdpRtcp       *serverudp.Server
	conn                *gortsplib.ConnServer
	parent              Parent

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

// New allocates a Client.
func New(
	rtspPort int,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	runOnConnect string,
	runOnConnectRestart bool,
	protocols map[gortsplib.StreamProtocol]struct{},
	wg *sync.WaitGroup,
	stats *stats.Stats,
	serverUdpRtp *serverudp.Server,
	serverUdpRtcp *serverudp.Server,
	nconn net.Conn,
	parent Parent) *Client {

	c := &Client{
		rtspPort:            rtspPort,
		readTimeout:         readTimeout,
		runOnConnect:        runOnConnect,
		runOnConnectRestart: runOnConnectRestart,
		protocols:           protocols,
		wg:                  wg,
		stats:               stats,
		serverUdpRtp:        serverUdpRtp,
		serverUdpRtcp:       serverUdpRtcp,
		conn: gortsplib.NewConnServer(gortsplib.ConnServerConf{
			Conn:            nconn,
			ReadTimeout:     readTimeout,
			WriteTimeout:    writeTimeout,
			ReadBufferCount: 2,
		}),
		parent:       parent,
		state:        stateInitial,
		streamTracks: make(map[int]*streamTrack),
		terminate:    make(chan struct{}),
	}

	atomic.AddInt64(c.stats.CountClients, 1)
	c.log("connected")

	c.wg.Add(1)
	go c.run()
	return c
}

// Close closes a Client.
func (c *Client) Close() {
	atomic.AddInt64(c.stats.CountClients, -1)
	close(c.terminate)
}

// IsSource implementes path.source.
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

var errStateTerminate = errors.New("terminate")
var errStateWaitingDescribe = errors.New("wait description")
var errStatePlay = errors.New("play")
var errStateRecord = errors.New("record")
var errStateInitial = errors.New("initial")

func (c *Client) run() {
	defer c.wg.Done()
	defer c.log("disconnected")

	var onConnectCmd *externalcmd.ExternalCmd
	if c.runOnConnect != "" {
		onConnectCmd = externalcmd.New(c.runOnConnect, c.runOnConnectRestart, externalcmd.Environment{
			Path: "",
			Port: strconv.FormatInt(int64(c.rtspPort), 10),
		})
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
}

type errAuthNotCritical struct {
	*base.Response
}

func (errAuthNotCritical) Error() string {
	return "auth not critical"
}

type errAuthCritical struct {
	*base.Response
}

func (errAuthCritical) Error() string {
	return "auth critical"
}

// Authenticate performs an authentication.
func (c *Client) Authenticate(authMethods []headers.AuthMethod, ips []interface{}, user string, pass string, req *base.Request) error {
	// validate ip
	if ips != nil {
		ip := c.ip()

		if !ipEqualOrInRange(ip, ips) {
			c.log("ERR: ip '%s' not allowed", ip)

			return errAuthCritical{&base.Response{
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

		err := c.authHelper.ValidateHeader(req.Header["Authorization"], req.Method, req.URL)
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

				return errAuthCritical{&base.Response{
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

				return errAuthNotCritical{&base.Response{
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

func (c *Client) checkState(allowed map[state]struct{}) error {
	if _, ok := allowed[c.state]; ok {
		return nil
	}

	var allowedList []state
	for s := range allowed {
		allowedList = append(allowedList, s)
	}
	return fmt.Errorf("client must be in state %v, while is in state %v",
		allowedList, c.state)
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

func (c *Client) handleRequest(req *base.Request) error {
	c.log(string(req.Method))

	cseq, ok := req.Header["CSeq"]
	if !ok || len(cseq) != 1 {
		c.writeResError(nil, base.StatusBadRequest, fmt.Errorf("cseq missing"))
		return errStateTerminate
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
		err := c.checkState(map[state]struct{}{
			stateInitial: {},
		})
		if err != nil {
			c.writeResError(cseq, base.StatusBadRequest, err)
			return errStateTerminate
		}

		basePath, ok := req.URL.BasePath()
		if !ok {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("unable to find base path (%s)", req.URL))
			return errStateTerminate
		}

		c.describeData = make(chan describeData)

		path, err := c.parent.OnClientDescribe(c, basePath, req)
		if err != nil {
			switch terr := err.(type) {
			case errAuthNotCritical:
				close(c.describeData)
				c.conn.WriteResponse(terr.Response)
				return nil

			case errAuthCritical:
				close(c.describeData)
				c.conn.WriteResponse(terr.Response)
				return errStateTerminate

			default:
				c.writeResError(cseq, base.StatusBadRequest, err)
				return errStateTerminate
			}
		}

		c.path = path
		c.state = stateWaitingDescribe
		c.describeCSeq = cseq
		c.describeUrl = req.URL.String()

		return errStateWaitingDescribe

	case base.ANNOUNCE:
		err := c.checkState(map[state]struct{}{
			stateInitial: {},
		})
		if err != nil {
			c.writeResError(cseq, base.StatusBadRequest, err)
			return errStateTerminate
		}

		ct, ok := req.Header["Content-Type"]
		if !ok || len(ct) != 1 {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("Content-Type header missing"))
			return errStateTerminate
		}

		if ct[0] != "application/sdp" {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("unsupported Content-Type '%s'", ct))
			return errStateTerminate
		}

		tracks, err := gortsplib.ReadTracks(req.Content)
		if err != nil {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("invalid SDP: %s", err))
			return errStateTerminate
		}

		if len(tracks) == 0 {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("no tracks defined"))
			return errStateTerminate
		}

		basePath, ok := req.URL.BasePath()
		if !ok {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("unable to find base path (%s)", req.URL))
			return errStateTerminate
		}

		path, err := c.parent.OnClientAnnounce(c, basePath, tracks, req)
		if err != nil {
			switch terr := err.(type) {
			case errAuthNotCritical:
				c.conn.WriteResponse(terr.Response)
				return nil

			case errAuthCritical:
				c.conn.WriteResponse(terr.Response)
				return errStateTerminate

			default:
				c.writeResError(cseq, base.StatusBadRequest, err)
				return errStateTerminate
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
			return errStateTerminate
		}

		if th.Delivery != nil && *th.Delivery == base.StreamDeliveryMulticast {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("multicast is not supported"))
			return errStateTerminate
		}

		basePath, controlPath, ok := req.URL.BasePathControlAttr()
		if !ok {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("unable to find control attribute (%s)", req.URL))
			return errStateTerminate
		}

		switch c.state {
		// play
		case stateInitial, statePrePlay:
			if th.Mode != nil && *th.Mode != headers.TransportModePlay {
				c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("transport header must contain mode=play or not contain a mode"))
				return errStateTerminate
			}

			if c.path != nil && basePath != c.path.Name() {
				c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.Name(), basePath))
				return errStateTerminate
			}

			if !strings.HasPrefix(controlPath, "trackID=") {
				c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("invalid control attribute (%s)", controlPath))
				return errStateTerminate
			}

			tmp, err := strconv.ParseInt(controlPath[len("trackID="):], 10, 64)
			if err != nil || tmp < 0 {
				c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("invalid track id (%s)", controlPath))
				return errStateTerminate
			}
			trackId := int(tmp)

			if _, ok := c.streamTracks[trackId]; ok {
				c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("track %d has already been setup", trackId))
				return errStateTerminate
			}

			// play with UDP
			if th.Protocol == gortsplib.StreamProtocolUDP {
				if _, ok := c.protocols[gortsplib.StreamProtocolUDP]; !ok {
					c.writeResError(cseq, base.StatusUnsupportedTransport, fmt.Errorf("UDP streaming is disabled"))
					return nil
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolUDP {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("can't receive tracks with different protocols"))
					return errStateTerminate
				}

				if th.ClientPorts == nil {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("transport header does not have valid client ports (%v)", req.Header["Transport"]))
					return errStateTerminate
				}

				path, err := c.parent.OnClientSetupPlay(c, basePath, trackId, req)
				if err != nil {
					switch terr := err.(type) {
					case errAuthNotCritical:
						c.conn.WriteResponse(terr.Response)
						return nil

					case errAuthCritical:
						c.conn.WriteResponse(terr.Response)
						return errStateTerminate

					default:
						c.writeResError(cseq, base.StatusBadRequest, err)
						return errStateTerminate
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
					Delivery: func() *base.StreamDelivery {
						v := base.StreamDeliveryUnicast
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
						"Session":   base.HeaderValue{sessionId},
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
					return errStateTerminate
				}

				path, err := c.parent.OnClientSetupPlay(c, basePath, trackId, req)
				if err != nil {
					switch terr := err.(type) {
					case errAuthNotCritical:
						c.conn.WriteResponse(terr.Response)
						return nil

					case errAuthCritical:
						c.conn.WriteResponse(terr.Response)
						return errStateTerminate

					default:
						c.writeResError(cseq, base.StatusBadRequest, err)
						return errStateTerminate
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
						"Session":   base.HeaderValue{sessionId},
					},
				})
				return nil
			}

		// record
		case statePreRecord:
			if th.Mode == nil || *th.Mode != headers.TransportModeRecord {
				c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("transport header does not contain mode=record"))
				return errStateTerminate
			}

			// after ANNOUNCE, c.path is already set
			if basePath != c.path.Name() {
				c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.Name(), basePath))
				return errStateTerminate
			}

			// record with UDP
			if th.Protocol == gortsplib.StreamProtocolUDP {
				if _, ok := c.protocols[gortsplib.StreamProtocolUDP]; !ok {
					c.writeResError(cseq, base.StatusUnsupportedTransport, fmt.Errorf("UDP streaming is disabled"))
					return nil
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolUDP {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("can't publish tracks with different protocols"))
					return errStateTerminate
				}

				if th.ClientPorts == nil {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("transport header does not have valid client ports (%s)", req.Header["Transport"]))
					return errStateTerminate
				}

				if len(c.streamTracks) >= c.path.SourceTrackCount() {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("all the tracks have already been setup"))
					return errStateTerminate
				}

				c.streamProtocol = gortsplib.StreamProtocolUDP
				c.streamTracks[len(c.streamTracks)] = &streamTrack{
					rtpPort:  (*th.ClientPorts)[0],
					rtcpPort: (*th.ClientPorts)[1],
				}

				th := &headers.Transport{
					Protocol: gortsplib.StreamProtocolUDP,
					Delivery: func() *base.StreamDelivery {
						v := base.StreamDeliveryUnicast
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
						"Session":   base.HeaderValue{sessionId},
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
					return errStateTerminate
				}

				interleavedIds := [2]int{len(c.streamTracks) * 2, 1 + len(c.streamTracks)*2}

				if th.InterleavedIds == nil {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("transport header does not contain the interleaved field"))
					return errStateTerminate
				}

				if (*th.InterleavedIds)[0] != interleavedIds[0] || (*th.InterleavedIds)[1] != interleavedIds[1] {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("wrong interleaved ids, expected %v, got %v", interleavedIds, *th.InterleavedIds))
					return errStateTerminate
				}

				if len(c.streamTracks) >= c.path.SourceTrackCount() {
					c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("all the tracks have already been setup"))
					return errStateTerminate
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
						"Session":   base.HeaderValue{sessionId},
					},
				})
				return nil
			}

		default:
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("client is in state '%s'", c.state))
			return errStateTerminate
		}

	case base.PLAY:
		err := c.checkState(map[state]struct{}{
			statePrePlay: {},
		})
		if err != nil {
			c.writeResError(cseq, base.StatusBadRequest, err)
			return errStateTerminate
		}

		basePath, ok := req.URL.BasePath()
		if !ok {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("unable to find base path (%s)", req.URL))
			return errStateTerminate
		}

		// path can end with a slash, remove it
		basePath = strings.TrimSuffix(basePath, "/")

		if basePath != c.path.Name() {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.Name(), basePath))
			return errStateTerminate
		}

		if len(c.streamTracks) == 0 {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("no tracks have been setup"))
			return errStateTerminate
		}

		// write response before setting state
		// otherwise, in case of TCP connections, RTP packets could be sent
		// before the response
		c.conn.WriteResponse(&base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"CSeq":    cseq,
				"Session": base.HeaderValue{sessionId},
			},
		})
		return errStatePlay

	case base.RECORD:
		err := c.checkState(map[state]struct{}{
			statePreRecord: {},
		})
		if err != nil {
			c.writeResError(cseq, base.StatusBadRequest, err)
			return errStateTerminate
		}

		basePath, ok := req.URL.BasePath()
		if !ok {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("unable to find base path (%s)", req.URL))
			return errStateTerminate
		}

		// path can end with a slash, remove it
		basePath = strings.TrimSuffix(basePath, "/")

		if basePath != c.path.Name() {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.Name(), basePath))
			return errStateTerminate
		}

		if len(c.streamTracks) != c.path.SourceTrackCount() {
			c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("not all tracks have been setup"))
			return errStateTerminate
		}

		c.conn.WriteResponse(&base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"CSeq":    cseq,
				"Session": base.HeaderValue{sessionId},
			},
		})
		return errStateRecord

	case base.PAUSE:
		err := c.checkState(map[state]struct{}{
			statePrePlay:   {},
			statePlay:      {},
			statePreRecord: {},
			stateRecord:    {},
		})
		if err != nil {
			c.writeResError(cseq, base.StatusBadRequest, err)
			return errStateTerminate
		}

		c.conn.WriteResponse(&base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"CSeq":    cseq,
				"Session": base.HeaderValue{sessionId},
			},
		})

		if c.state == statePlay || c.state == stateRecord {
			return errStateInitial
		}
		return nil

	case base.TEARDOWN:
		// close connection silently
		return errStateTerminate

	default:
		c.writeResError(cseq, base.StatusBadRequest, fmt.Errorf("unhandled method '%s'", req.Method))
		return errStateTerminate
	}
}

func (c *Client) runInitial() bool {
	readerDone := make(chan error)
	go func() {
		for {
			req, err := c.conn.ReadRequest()
			if err != nil {
				readerDone <- err
				return
			}

			err = c.handleRequest(req)
			if err != nil {
				readerDone <- err
				return
			}
		}
	}()

	select {
	case err := <-readerDone:
		switch err {
		case errStateWaitingDescribe:
			return c.runWaitingDescribe()

		case errStatePlay:
			return c.runPlay()

		case errStateRecord:
			return c.runRecord()

		default:
			c.conn.Close()
			if err != io.EOF && err != errStateTerminate {
				c.log("ERR: %s", err)
			}

			c.parent.OnClientClose(c)
			<-c.terminate
			return false
		}

	case <-c.terminate:
		c.conn.Close()
		<-readerDone
		return false
	}
}

func (c *Client) runWaitingDescribe() bool {
	select {
	case res := <-c.describeData:
		c.path.OnClientRemove(c)
		c.path = nil

		close(c.describeData)

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
		ch := c.describeData
		go func() {
			for range ch {
			}
		}()

		c.path.OnClientRemove(c)
		c.path = nil

		close(c.describeData)

		c.conn.Close()
		return false
	}
}

func (c *Client) runPlay() bool {
	if c.streamProtocol == gortsplib.StreamProtocolTCP {
		c.tcpFrame = make(chan *base.InterleavedFrame)
	}

	// start sending frames only after replying to the PLAY request
	c.state = statePlay
	c.path.OnClientPlay(c)

	c.log("is reading from path '%s', %d %s with %s", c.path.Name(), len(c.streamTracks), func() string {
		if len(c.streamTracks) == 1 {
			return "track"
		}
		return "tracks"
	}(), c.streamProtocol)

	var onReadCmd *externalcmd.ExternalCmd
	if c.path.Conf().RunOnRead != "" {
		onReadCmd = externalcmd.New(c.path.Conf().RunOnRead, c.path.Conf().RunOnReadRestart, externalcmd.Environment{
			Path: c.path.Name(),
			Port: strconv.FormatInt(int64(c.rtspPort), 10),
		})
	}

	var ret bool
	if c.streamProtocol == gortsplib.StreamProtocolUDP {
		ret = c.runPlayUDP()
	} else {
		ret = c.runPlayTCP()
	}

	if onReadCmd != nil {
		onReadCmd.Close()
	}

	return ret
}

func (c *Client) runPlayUDP() bool {
	readerDone := make(chan error)
	go func() {
		for {
			req, err := c.conn.ReadRequest()
			if err != nil {
				readerDone <- err
				return
			}

			err = c.handleRequest(req)
			if err != nil {
				readerDone <- err
				return
			}
		}
	}()

	select {
	case err := <-readerDone:
		if err == errStateInitial {
			c.state = statePrePlay
			c.path.OnClientPause(c)
			return true

		} else {
			c.path.OnClientRemove(c)
			c.path = nil

			c.conn.Close()
			if err != io.EOF && err != errStateTerminate {
				c.log("ERR: %s", err)
			}

			c.parent.OnClientClose(c)
			<-c.terminate
			return false
		}

	case <-c.terminate:
		c.path.OnClientRemove(c)
		c.path = nil

		c.conn.Close()
		<-readerDone
		return false
	}
}

func (c *Client) runPlayTCP() bool {
	readRequest := make(chan readRequestPair)
	defer close(readRequest)

	readerDone := make(chan error)
	go func() {
		for {
			recv, err := c.conn.ReadFrameTCPOrRequest(false)
			if err != nil {
				readerDone <- err
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
					readerDone <- err
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

		case err := <-readerDone:
			if err == errStateInitial {
				ch := c.tcpFrame
				go func() {
					for range ch {
					}
				}()

				c.state = statePrePlay
				c.path.OnClientPause(c)

				close(c.tcpFrame)
				return true

			} else {
				ch := c.tcpFrame
				go func() {
					for range ch {
					}
				}()

				c.path.OnClientRemove(c)
				c.path = nil

				close(c.tcpFrame)

				c.conn.Close()
				if err != io.EOF && err != errStateTerminate {
					c.log("ERR: %s", err)
				}

				c.parent.OnClientClose(c)
				<-c.terminate
				return false
			}

		case frame := <-c.tcpFrame:
			c.conn.WriteFrameTCP(frame.TrackId, frame.StreamType, frame.Content)

		case <-c.terminate:
			go func() {
				for req := range readRequest {
					req.res <- fmt.Errorf("terminated")
				}
			}()

			ch := c.tcpFrame
			go func() {
				for range ch {
				}
			}()

			c.path.OnClientRemove(c)
			c.path = nil

			close(c.tcpFrame)

			c.conn.Close()
			<-readerDone
			return false
		}
	}
}

func (c *Client) runRecord() bool {
	c.state = stateRecord
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
		onPublishCmd = externalcmd.New(c.path.Conf().RunOnPublish, c.path.Conf().RunOnPublishRestart, externalcmd.Environment{
			Path: c.path.Name(),
			Port: strconv.FormatInt(int64(c.rtspPort), 10),
		})
	}

	var ret bool
	if c.streamProtocol == gortsplib.StreamProtocolUDP {
		ret = c.runRecordUDP()
	} else {
		ret = c.runRecordTCP()
	}

	if onPublishCmd != nil {
		onPublishCmd.Close()
	}

	return ret
}

func (c *Client) runRecordUDP() bool {
	readerDone := make(chan error)
	go func() {
		for {
			req, err := c.conn.ReadRequest()
			if err != nil {
				readerDone <- err
				return
			}

			err = c.handleRequest(req)
			if err != nil {
				readerDone <- err
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
		case err := <-readerDone:
			if err == errStateInitial {
				for _, track := range c.streamTracks {
					c.serverUdpRtp.RemovePublisher(c.ip(), track.rtpPort, c)
					c.serverUdpRtcp.RemovePublisher(c.ip(), track.rtcpPort, c)
				}

				c.state = statePreRecord
				c.path.OnClientPause(c)

				return true

			} else {
				for _, track := range c.streamTracks {
					c.serverUdpRtp.RemovePublisher(c.ip(), track.rtpPort, c)
					c.serverUdpRtcp.RemovePublisher(c.ip(), track.rtcpPort, c)
				}

				c.path.OnClientRemove(c)
				c.path = nil

				c.conn.Close()
				if err != io.EOF && err != errStateTerminate {
					c.log("ERR: %s", err)
				}

				c.parent.OnClientClose(c)
				<-c.terminate
				return false
			}

		case <-checkStreamTicker.C:
			now := time.Now()

			for _, lastUnix := range c.udpLastFrameTimes {
				last := time.Unix(atomic.LoadInt64(lastUnix), 0)

				if now.Sub(last) >= c.readTimeout {
					for _, track := range c.streamTracks {
						c.serverUdpRtp.RemovePublisher(c.ip(), track.rtpPort, c)
						c.serverUdpRtcp.RemovePublisher(c.ip(), track.rtcpPort, c)
					}

					c.log("ERR: no packets received recently (maybe there's a firewall/NAT in between)")
					c.conn.Close()
					<-readerDone

					c.path.OnClientRemove(c)
					c.path = nil

					c.parent.OnClientClose(c)
					<-c.terminate

					return false
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
			for _, track := range c.streamTracks {
				c.serverUdpRtp.RemovePublisher(c.ip(), track.rtpPort, c)
				c.serverUdpRtcp.RemovePublisher(c.ip(), track.rtcpPort, c)
			}

			c.conn.Close()
			<-readerDone

			c.path.OnClientRemove(c)
			c.path = nil

			return false
		}
	}
}

func (c *Client) runRecordTCP() bool {
	readRequest := make(chan readRequestPair)
	defer close(readRequest)

	readerDone := make(chan error)
	go func() {
		for {
			recv, err := c.conn.ReadFrameTCPOrRequest(true)
			if err != nil {
				readerDone <- err
				return
			}

			switch recvt := recv.(type) {
			case *base.InterleavedFrame:
				if recvt.TrackId >= len(c.streamTracks) {
					readerDone <- fmt.Errorf("invalid track id '%d'", recvt.TrackId)
					return
				}

				c.rtcpReceivers[recvt.TrackId].OnFrame(recvt.StreamType, recvt.Content)
				c.path.OnFrame(recvt.TrackId, recvt.StreamType, recvt.Content)

			case *base.Request:
				err := c.handleRequest(recvt)
				if err != nil {
					readerDone <- err
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

		case err := <-readerDone:
			if err == errStateInitial {
				c.state = statePreRecord
				c.path.OnClientPause(c)

				return true

			} else {
				c.path.OnClientRemove(c)
				c.path = nil

				c.conn.Close()
				if err != io.EOF && err != errStateTerminate {
					c.log("ERR: %s", err)
				}

				c.parent.OnClientClose(c)
				<-c.terminate

				return false
			}

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
			<-readerDone

			c.path.OnClientRemove(c)
			c.path = nil

			return false
		}
	}
}

// OnUdpPublisherFrame implements serverudp.Publisher.
func (c *Client) OnUdpPublisherFrame(trackId int, streamType base.StreamType, buf []byte) {
	atomic.StoreInt64(c.udpLastFrameTimes[trackId], time.Now().Unix())

	c.rtcpReceivers[trackId].OnFrame(streamType, buf)
	c.path.OnFrame(trackId, streamType, buf)
}

// OnReaderFrame implements path.Reader.
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

// OnPathDescribeData is called by path.Path.
func (c *Client) OnPathDescribeData(sdp []byte, redirect string, err error) {
	c.describeData <- describeData{sdp, redirect, err}
}
