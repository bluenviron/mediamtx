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
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/serverudp"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

const (
	checkStreamInterval    = 5 * time.Second
	receiverReportInterval = 10 * time.Second
	sessionID              = "12345678"
	pauseAfterAuthError    = 2 * time.Second
)

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
	statePrePlay
	statePlay
	statePreRecord
	stateRecord
)

func (s state) String() string {
	switch s {
	case stateInitial:
		return "initial"
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
	Log(logger.Level, string, ...interface{})
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
	serverUDPRtp        *serverudp.Server
	serverUDPRtcp       *serverudp.Server
	conn                *gortsplib.ServerConn
	parent              Parent

	state             state
	path              Path
	authUser          string
	authPass          string
	authHelper        *auth.Server
	authFailures      int
	streamProtocol    gortsplib.StreamProtocol
	streamTracks      map[int]*streamTrack
	rtcpReceivers     map[int]*rtcpreceiver.RtcpReceiver
	udpLastFrameTimes []*int64
	onReadCmd         *externalcmd.Cmd
	onPublishCmd      *externalcmd.Cmd

	// in
	describeData chan describeData // from path
	terminate    chan struct{}

	backgroundRecordTerminate chan struct{}
	backgroundRecordDone      chan struct{}
}

// New allocates a Client.
func New(
	isTLS bool,
	rtspPort int,
	readTimeout time.Duration,
	runOnConnect string,
	runOnConnectRestart bool,
	protocols map[gortsplib.StreamProtocol]struct{},
	wg *sync.WaitGroup,
	stats *stats.Stats,
	serverUDPRtp *serverudp.Server,
	serverUDPRtcp *serverudp.Server,
	conn *gortsplib.ServerConn,
	parent Parent) *Client {

	c := &Client{
		rtspPort:            rtspPort,
		readTimeout:         readTimeout,
		runOnConnect:        runOnConnect,
		runOnConnectRestart: runOnConnectRestart,
		protocols:           protocols,
		wg:                  wg,
		stats:               stats,
		serverUDPRtp:        serverUDPRtp,
		serverUDPRtcp:       serverUDPRtcp,
		conn:                conn,
		parent:              parent,
		state:               stateInitial,
		streamTracks:        make(map[int]*streamTrack),
		rtcpReceivers:       make(map[int]*rtcpreceiver.RtcpReceiver),
		terminate:           make(chan struct{}),
	}

	atomic.AddInt64(c.stats.CountClients, 1)
	c.log(logger.Info, "connected (%s)", func() string {
		if isTLS {
			return "encrypted"
		}
		return "plain"
	}())

	c.wg.Add(1)
	go c.run()
	return c
}

// Close closes a Client.
func (c *Client) Close() {
	atomic.AddInt64(c.stats.CountClients, -1)
	close(c.terminate)
}

// IsSource implements path.source.
func (c *Client) IsSource() {}

func (c *Client) log(level logger.Level, format string, args ...interface{}) {
	c.parent.Log(level, "[client %s] "+format, append([]interface{}{c.conn.NetConn().RemoteAddr().String()}, args...)...)
}

func (c *Client) ip() net.IP {
	return c.conn.NetConn().RemoteAddr().(*net.TCPAddr).IP
}

func (c *Client) zone() string {
	return c.conn.NetConn().RemoteAddr().(*net.TCPAddr).Zone
}

var errTerminated = errors.New("terminated")

func (c *Client) run() {
	defer c.wg.Done()
	defer c.log(logger.Info, "disconnected")

	if c.runOnConnect != "" {
		onConnectCmd := externalcmd.New(c.runOnConnect, c.runOnConnectRestart, externalcmd.Environment{
			Path: "",
			Port: strconv.FormatInt(int64(c.rtspPort), 10),
		})
		defer onConnectCmd.Close()
	}

	onRequest := func(req *base.Request) {
		c.log(logger.Debug, "[c->s] %v", req)
	}

	onResponse := func(res *base.Response) {
		c.log(logger.Debug, "[s->c] %v", res)
	}

	onDescribe := func(req *base.Request) (*base.Response, error) {
		err := c.checkState(map[state]struct{}{
			stateInitial: {},
		})
		if err != nil {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, err
		}

		basePath, ok := req.URL.BasePath()
		if !ok {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, fmt.Errorf("unable to find base path (%s)", req.URL)
		}

		c.describeData = make(chan describeData)

		path, err := c.parent.OnClientDescribe(c, basePath, req)
		if err != nil {
			switch terr := err.(type) {
			case errAuthNotCritical:
				return terr.Response, nil

			case errAuthCritical:
				// wait some seconds to stop brute force attacks
				t := time.NewTimer(pauseAfterAuthError)
				defer t.Stop()
				select {
				case <-t.C:
				case <-c.terminate:
				}

				return terr.Response, errTerminated

			default:
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, err
			}
		}

		c.path = path

		select {
		case res := <-c.describeData:
			c.path.OnClientRemove(c)
			c.path = nil

			if res.err != nil {
				c.log(logger.Info, "no one is publishing to path '%s'", basePath)
				return &base.Response{
					StatusCode: base.StatusNotFound,
				}, nil
			}

			if res.redirect != "" {
				return &base.Response{
					StatusCode: base.StatusMovedPermanently,
					Header: base.Header{
						"Location": base.HeaderValue{res.redirect},
					},
				}, nil
			}

			return &base.Response{
				StatusCode: base.StatusOK,
				Header: base.Header{
					"Content-Base": base.HeaderValue{req.URL.String() + "/"},
					"Content-Type": base.HeaderValue{"application/sdp"},
				},
				Content: res.sdp,
			}, nil

		case <-c.terminate:
			ch := c.describeData
			go func() {
				for range ch {
				}
			}()

			c.path.OnClientRemove(c)
			c.path = nil

			close(c.describeData)

			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, errTerminated
		}
	}

	onAnnounce := func(req *base.Request, tracks gortsplib.Tracks) (*base.Response, error) {
		err := c.checkState(map[state]struct{}{
			stateInitial: {},
		})
		if err != nil {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, err
		}

		basePath, ok := req.URL.BasePath()
		if !ok {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, fmt.Errorf("unable to find base path (%s)", req.URL)
		}

		path, err := c.parent.OnClientAnnounce(c, basePath, tracks, req)
		if err != nil {
			switch terr := err.(type) {
			case errAuthNotCritical:
				return terr.Response, nil

			case errAuthCritical:
				// wait some seconds to stop brute force attacks
				t := time.NewTimer(pauseAfterAuthError)
				defer t.Stop()
				select {
				case <-t.C:
				case <-c.terminate:
				}

				return terr.Response, errTerminated

			default:
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, err
			}
		}

		for trackID, t := range tracks {
			clockRate, _ := t.ClockRate()
			c.rtcpReceivers[trackID] = rtcpreceiver.New(nil, clockRate)
		}

		c.path = path
		c.state = statePreRecord

		return &base.Response{
			StatusCode: base.StatusOK,
		}, nil
	}

	onSetup := func(req *base.Request, th *headers.Transport) (*base.Response, error) {
		if th.Delivery != nil && *th.Delivery == base.StreamDeliveryMulticast {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, fmt.Errorf("multicast is not supported")
		}

		basePath, controlPath, ok := req.URL.BasePathControlAttr()
		if !ok {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, fmt.Errorf("unable to find control attribute (%s)", req.URL)
		}

		switch c.state {
		// play
		case stateInitial, statePrePlay:
			if th.Mode != nil && *th.Mode != headers.TransportModePlay {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("transport header must contain mode=play or not contain a mode")
			}

			if c.path != nil && basePath != c.path.Name() {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.Name(), basePath)
			}

			if !strings.HasPrefix(controlPath, "trackID=") {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("invalid control attribute (%s)", controlPath)
			}

			tmp, err := strconv.ParseInt(controlPath[len("trackID="):], 10, 64)
			if err != nil || tmp < 0 {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("invalid track id (%s)", controlPath)
			}
			trackID := int(tmp)

			if _, ok := c.streamTracks[trackID]; ok {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("track %d has already been setup", trackID)
			}

			// play with UDP
			if th.Protocol == gortsplib.StreamProtocolUDP {
				if _, ok := c.protocols[gortsplib.StreamProtocolUDP]; !ok {
					return &base.Response{
						StatusCode: base.StatusUnsupportedTransport,
					}, nil
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolUDP {
					return &base.Response{
						StatusCode: base.StatusBadRequest,
					}, fmt.Errorf("can't receive tracks with different protocols")
				}

				if th.ClientPorts == nil {
					return &base.Response{
						StatusCode: base.StatusBadRequest,
					}, fmt.Errorf("transport header does not have valid client ports (%v)", req.Header["Transport"])
				}

				path, err := c.parent.OnClientSetupPlay(c, basePath, trackID, req)
				if err != nil {
					switch terr := err.(type) {
					case errAuthNotCritical:
						return terr.Response, nil

					case errAuthCritical:
						// wait some seconds to stop brute force attacks
						t := time.NewTimer(pauseAfterAuthError)
						defer t.Stop()
						select {
						case <-t.C:
						case <-c.terminate:
						}

						return terr.Response, errTerminated

					default:
						return &base.Response{
							StatusCode: base.StatusBadRequest,
						}, err
					}
				}

				c.path = path
				c.state = statePrePlay

				c.streamProtocol = gortsplib.StreamProtocolUDP
				c.streamTracks[trackID] = &streamTrack{
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
					ServerPorts: &[2]int{c.serverUDPRtp.Port(), c.serverUDPRtcp.Port()},
				}

				return &base.Response{
					StatusCode: base.StatusOK,
					Header: base.Header{
						"Transport": th.Write(),
						"Session":   base.HeaderValue{sessionID},
					},
				}, nil
			}

			// play with TCP

			if _, ok := c.protocols[gortsplib.StreamProtocolTCP]; !ok {
				return &base.Response{
					StatusCode: base.StatusUnsupportedTransport,
				}, nil
			}

			if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolTCP {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("can't receive tracks with different protocols")
			}

			path, err := c.parent.OnClientSetupPlay(c, basePath, trackID, req)
			if err != nil {
				switch terr := err.(type) {
				case errAuthNotCritical:
					return terr.Response, nil

				case errAuthCritical:
					// wait some seconds to stop brute force attacks
					t := time.NewTimer(pauseAfterAuthError)
					defer t.Stop()
					select {
					case <-t.C:
					case <-c.terminate:
					}

					return terr.Response, errTerminated

				default:
					return &base.Response{
						StatusCode: base.StatusBadRequest,
					}, err
				}
			}

			c.path = path
			c.state = statePrePlay

			c.streamProtocol = gortsplib.StreamProtocolTCP
			c.streamTracks[trackID] = &streamTrack{
				rtpPort:  0,
				rtcpPort: 0,
			}

			interleavedIds := [2]int{trackID * 2, (trackID * 2) + 1}

			th := &headers.Transport{
				Protocol:       gortsplib.StreamProtocolTCP,
				InterleavedIds: &interleavedIds,
			}

			return &base.Response{
				StatusCode: base.StatusOK,
				Header: base.Header{
					"Transport": th.Write(),
					"Session":   base.HeaderValue{sessionID},
				},
			}, nil

		// record
		case statePreRecord:
			if th.Mode == nil || *th.Mode != headers.TransportModeRecord {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("transport header does not contain mode=record")
			}

			// after ANNOUNCE, c.path is already set
			if basePath != c.path.Name() {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.Name(), basePath)
			}

			// record with UDP
			if th.Protocol == gortsplib.StreamProtocolUDP {
				if _, ok := c.protocols[gortsplib.StreamProtocolUDP]; !ok {
					return &base.Response{
						StatusCode: base.StatusUnsupportedTransport,
					}, nil
				}

				if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolUDP {
					return &base.Response{
						StatusCode: base.StatusBadRequest,
					}, fmt.Errorf("can't publish tracks with different protocols")
				}

				if th.ClientPorts == nil {
					return &base.Response{
						StatusCode: base.StatusBadRequest,
					}, fmt.Errorf("transport header does not have valid client ports (%s)", req.Header["Transport"])
				}

				if len(c.streamTracks) >= c.path.SourceTrackCount() {
					return &base.Response{
						StatusCode: base.StatusBadRequest,
					}, fmt.Errorf("all the tracks have already been setup")
				}

				c.streamProtocol = gortsplib.StreamProtocolUDP
				trackID := len(c.streamTracks)
				c.streamTracks[trackID] = &streamTrack{
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
					ServerPorts: &[2]int{c.serverUDPRtp.Port(), c.serverUDPRtcp.Port()},
				}

				return &base.Response{
					StatusCode: base.StatusOK,
					Header: base.Header{
						"Transport": th.Write(),
						"Session":   base.HeaderValue{sessionID},
					},
				}, nil
			}

			// record with TCP
			if _, ok := c.protocols[gortsplib.StreamProtocolTCP]; !ok {
				return &base.Response{
					StatusCode: base.StatusUnsupportedTransport,
				}, nil
			}

			if len(c.streamTracks) > 0 && c.streamProtocol != gortsplib.StreamProtocolTCP {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("can't publish tracks with different protocols")
			}

			interleavedIds := [2]int{len(c.streamTracks) * 2, 1 + len(c.streamTracks)*2}

			if th.InterleavedIds == nil {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("transport header does not contain the interleaved field")
			}

			if (*th.InterleavedIds)[0] != interleavedIds[0] || (*th.InterleavedIds)[1] != interleavedIds[1] {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("wrong interleaved ids, expected %v, got %v", interleavedIds, *th.InterleavedIds)
			}

			if len(c.streamTracks) >= c.path.SourceTrackCount() {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("all the tracks have already been setup")
			}

			c.streamProtocol = gortsplib.StreamProtocolTCP
			trackID := len(c.streamTracks)
			c.streamTracks[trackID] = &streamTrack{
				rtpPort:  0,
				rtcpPort: 0,
			}

			ht := &headers.Transport{
				Protocol:       gortsplib.StreamProtocolTCP,
				InterleavedIds: &interleavedIds,
			}

			return &base.Response{
				StatusCode: base.StatusOK,
				Header: base.Header{
					"Transport": ht.Write(),
					"Session":   base.HeaderValue{sessionID},
				},
			}, nil

		default:
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, fmt.Errorf("client is in state '%s'", c.state)
		}
	}

	onPlay := func(req *base.Request) (*base.Response, error) {
		// play can be sent twice, allow calling it even if we're already playing
		err := c.checkState(map[state]struct{}{
			statePrePlay: {},
			statePlay:    {},
		})
		if err != nil {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, err
		}

		if c.state == statePrePlay {
			basePath, ok := req.URL.BasePath()
			if !ok {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("unable to find base path (%s)", req.URL)
			}

			// path can end with a slash, remove it
			basePath = strings.TrimSuffix(basePath, "/")

			if basePath != c.path.Name() {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.Name(), basePath)
			}

			if len(c.streamTracks) == 0 {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("no tracks have been setup")
			}
		}

		c.startPlay()

		return &base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"Session": base.HeaderValue{sessionID},
			},
		}, nil
	}

	onRecord := func(req *base.Request) (*base.Response, error) {
		err := c.checkState(map[state]struct{}{
			statePreRecord: {},
		})
		if err != nil {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, err
		}

		basePath, ok := req.URL.BasePath()
		if !ok {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, fmt.Errorf("unable to find base path (%s)", req.URL)
		}

		// path can end with a slash, remove it
		basePath = strings.TrimSuffix(basePath, "/")

		if basePath != c.path.Name() {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.Name(), basePath)
		}

		if len(c.streamTracks) != c.path.SourceTrackCount() {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, fmt.Errorf("not all tracks have been setup")
		}

		c.startRecord()

		return &base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"Session": base.HeaderValue{sessionID},
			},
		}, nil
	}

	onPause := func(req *base.Request) (*base.Response, error) {
		err := c.checkState(map[state]struct{}{
			statePrePlay:   {},
			statePlay:      {},
			statePreRecord: {},
			stateRecord:    {},
		})
		if err != nil {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, err
		}

		switch c.state {
		case statePlay:
			if c.streamProtocol == gortsplib.StreamProtocolTCP {
				c.conn.EnableFrames(false)
			}
			c.stopPlay()
			c.state = statePrePlay

		case stateRecord:
			if c.streamProtocol == gortsplib.StreamProtocolTCP {
				c.conn.EnableFrames(false)
				c.conn.EnableReadTimeout(false)
			}
			c.stopRecord()
			c.state = statePreRecord
		}

		return &base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"Session": base.HeaderValue{sessionID},
			},
		}, nil
	}

	onFrame := func(trackID int, streamType gortsplib.StreamType, content []byte) {
		if c.state == stateRecord {
			if trackID >= len(c.streamTracks) {
				return
			}

			c.rtcpReceivers[trackID].ProcessFrame(time.Now(), streamType, content)
			c.path.OnFrame(trackID, streamType, content)
		}
	}

	readDone := c.conn.Read(gortsplib.ServerConnReadHandlers{
		OnRequest:  onRequest,
		OnResponse: onResponse,
		OnDescribe: onDescribe,
		OnAnnounce: onAnnounce,
		OnSetup:    onSetup,
		OnPlay:     onPlay,
		OnRecord:   onRecord,
		OnPause:    onPause,
		OnFrame:    onFrame,
	})

	select {
	case err := <-readDone:
		c.conn.Close()
		if err != io.EOF && err != gortsplib.ErrServerTeardown && err != errTerminated {
			c.log(logger.Info, "ERR: %s", err)
		}

		switch c.state {
		case statePlay:
			c.stopPlay()

		case stateRecord:
			c.stopRecord()
		}

		if c.path != nil {
			c.path.OnClientRemove(c)
			c.path = nil
		}

		c.parent.OnClientClose(c)
		<-c.terminate

	case <-c.terminate:
		c.conn.Close()
		<-readDone

		switch c.state {
		case statePlay:
			c.stopPlay()

		case stateRecord:
			c.stopRecord()
		}

		if c.path != nil {
			c.path.OnClientRemove(c)
			c.path = nil
		}
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
			c.log(logger.Info, "ERR: ip '%s' not allowed", ip)

			return errAuthCritical{&base.Response{
				StatusCode: base.StatusUnauthorized,
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
			c.authFailures++

			// vlc with login prompt sends 4 requests:
			// 1) without credentials
			// 2) with password but without username
			// 3) without credentials
			// 4) with password and username
			// therefore we must allow up to 3 failures
			if c.authFailures > 3 {
				c.log(logger.Info, "ERR: unauthorized: %s", err)

				return errAuthCritical{&base.Response{
					StatusCode: base.StatusUnauthorized,
					Header: base.Header{
						"WWW-Authenticate": c.authHelper.GenerateHeader(),
					},
				}}
			}

			if c.authFailures > 1 {
				c.log(logger.Debug, "WARN: unauthorized: %s", err)
			}

			return errAuthNotCritical{&base.Response{
				StatusCode: base.StatusUnauthorized,
				Header: base.Header{
					"WWW-Authenticate": c.authHelper.GenerateHeader(),
				},
			}}
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

func (c *Client) startPlay() {
	c.state = statePlay
	c.path.OnClientPlay(c)

	c.log(logger.Info, "is reading from path '%s', %d %s with %s", c.path.Name(), len(c.streamTracks), func() string {
		if len(c.streamTracks) == 1 {
			return "track"
		}
		return "tracks"
	}(), c.streamProtocol)

	if c.path.Conf().RunOnRead != "" {
		c.onReadCmd = externalcmd.New(c.path.Conf().RunOnRead, c.path.Conf().RunOnReadRestart, externalcmd.Environment{
			Path: c.path.Name(),
			Port: strconv.FormatInt(int64(c.rtspPort), 10),
		})
	}

	if c.streamProtocol == gortsplib.StreamProtocolTCP {
		c.conn.EnableFrames(true)
	}
}

func (c *Client) stopPlay() {
	if c.path.Conf().RunOnRead != "" {
		c.onReadCmd.Close()
	}
}

func (c *Client) startRecord() {
	c.state = stateRecord
	c.path.OnClientRecord(c)

	c.log(logger.Info, "is publishing to path '%s', %d %s with %s", c.path.Name(), len(c.streamTracks), func() string {
		if len(c.streamTracks) == 1 {
			return "track"
		}
		return "tracks"
	}(), c.streamProtocol)

	if c.streamProtocol == gortsplib.StreamProtocolUDP {
		c.udpLastFrameTimes = make([]*int64, len(c.streamTracks))
		for trackID := range c.streamTracks {
			v := time.Now().Unix()
			c.udpLastFrameTimes[trackID] = &v
		}

		for trackID, track := range c.streamTracks {
			c.serverUDPRtp.AddPublisher(c.ip(), track.rtpPort, c, trackID)
			c.serverUDPRtcp.AddPublisher(c.ip(), track.rtcpPort, c, trackID)
		}

		// open the firewall by sending packets to the counterpart
		for _, track := range c.streamTracks {
			c.serverUDPRtp.Write(
				[]byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
				&net.UDPAddr{
					IP:   c.ip(),
					Zone: c.zone(),
					Port: track.rtpPort,
				})

			c.serverUDPRtcp.Write(
				[]byte{0x80, 0xc9, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00},
				&net.UDPAddr{
					IP:   c.ip(),
					Zone: c.zone(),
					Port: track.rtcpPort,
				})
		}
	}

	if c.path.Conf().RunOnPublish != "" {
		c.onPublishCmd = externalcmd.New(c.path.Conf().RunOnPublish, c.path.Conf().RunOnPublishRestart, externalcmd.Environment{
			Path: c.path.Name(),
			Port: strconv.FormatInt(int64(c.rtspPort), 10),
		})
	}

	c.backgroundRecordTerminate = make(chan struct{})
	c.backgroundRecordDone = make(chan struct{})

	if c.streamProtocol == gortsplib.StreamProtocolUDP {
		go c.backgroundRecordUDP()
	} else {
		c.conn.EnableFrames(true)
		c.conn.EnableReadTimeout(true)
		go c.backgroundRecordTCP()
	}
}

func (c *Client) stopRecord() {
	close(c.backgroundRecordTerminate)
	<-c.backgroundRecordDone

	if c.streamProtocol == gortsplib.StreamProtocolUDP {
		for _, track := range c.streamTracks {
			c.serverUDPRtp.RemovePublisher(c.ip(), track.rtpPort, c)
			c.serverUDPRtcp.RemovePublisher(c.ip(), track.rtcpPort, c)
		}
	}

	if c.path.Conf().RunOnPublish != "" {
		c.onPublishCmd.Close()
	}
}

func (c *Client) backgroundRecordUDP() {
	defer close(c.backgroundRecordDone)

	checkStreamTicker := time.NewTicker(checkStreamInterval)
	defer checkStreamTicker.Stop()

	receiverReportTicker := time.NewTicker(receiverReportInterval)
	defer receiverReportTicker.Stop()

	for {
		select {
		case <-checkStreamTicker.C:
			now := time.Now()

			for _, lastUnix := range c.udpLastFrameTimes {
				last := time.Unix(atomic.LoadInt64(lastUnix), 0)

				if now.Sub(last) >= c.readTimeout {
					c.log(logger.Info, "ERR: no UDP packets received recently (maybe there's a firewall/NAT in between)")
					c.conn.Close()
					return
				}
			}

		case <-receiverReportTicker.C:
			now := time.Now()
			for trackID := range c.streamTracks {
				r := c.rtcpReceivers[trackID].Report(now)
				c.serverUDPRtcp.Write(r, &net.UDPAddr{
					IP:   c.ip(),
					Zone: c.zone(),
					Port: c.streamTracks[trackID].rtcpPort,
				})
			}

		case <-c.backgroundRecordTerminate:
			return
		}
	}
}

func (c *Client) backgroundRecordTCP() {
	defer close(c.backgroundRecordDone)

	receiverReportTicker := time.NewTicker(receiverReportInterval)
	defer receiverReportTicker.Stop()

	for {
		select {
		case <-receiverReportTicker.C:
			now := time.Now()
			for trackID := range c.streamTracks {
				r := c.rtcpReceivers[trackID].Report(now)
				c.conn.WriteFrame(trackID, gortsplib.StreamTypeRtcp, r)
			}

		case <-c.backgroundRecordTerminate:
			return
		}
	}
}

// OnUDPPublisherFrame implements serverudp.Publisher.
func (c *Client) OnUDPPublisherFrame(trackID int, streamType base.StreamType, buf []byte) {
	now := time.Now()
	atomic.StoreInt64(c.udpLastFrameTimes[trackID], now.Unix())
	c.rtcpReceivers[trackID].ProcessFrame(now, streamType, buf)
	c.path.OnFrame(trackID, streamType, buf)
}

// OnReaderFrame implements path.Reader.
func (c *Client) OnReaderFrame(trackID int, streamType base.StreamType, buf []byte) {
	track, ok := c.streamTracks[trackID]
	if !ok {
		return
	}

	if c.streamProtocol == gortsplib.StreamProtocolUDP {
		if streamType == gortsplib.StreamTypeRtp {
			c.serverUDPRtp.Write(buf, &net.UDPAddr{
				IP:   c.ip(),
				Zone: c.zone(),
				Port: track.rtpPort,
			})

		} else {
			c.serverUDPRtcp.Write(buf, &net.UDPAddr{
				IP:   c.ip(),
				Zone: c.zone(),
				Port: track.rtcpPort,
			})
		}

	} else {
		c.conn.WriteFrame(trackID, streamType, buf)
	}
}

// OnPathDescribeData is called by path.Path.
func (c *Client) OnPathDescribeData(sdp []byte, redirect string, err error) {
	c.describeData <- describeData{sdp, redirect, err}
}
