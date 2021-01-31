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

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

const (
	sessionID           = "12345678"
	pauseAfterAuthError = 2 * time.Second
)

// ErrNoOnePublishing is a "no one is publishing" error.
type ErrNoOnePublishing struct {
	PathName string
}

// Error implements the error interface.
func (e ErrNoOnePublishing) Error() string {
	return fmt.Sprintf("no one is publishing to path '%s'", e.PathName)
}

// DescribeRes is a client describe response.
type DescribeRes struct {
	SDP      []byte
	Redirect string
	Err      error
}

// DescribeReq is a client describe request.
type DescribeReq struct {
	Client   *Client
	PathName string
	Req      *base.Request
	Res      chan DescribeRes
}

// AnnounceRes is a client announce response.
type AnnounceRes struct {
	Path Path
	Err  error
}

// AnnounceReq is a client announce request.
type AnnounceReq struct {
	Client   *Client
	PathName string
	Tracks   gortsplib.Tracks
	Req      *base.Request
	Res      chan AnnounceRes
}

// SetupPlayRes is a setup/play response.
type SetupPlayRes struct {
	Path Path
	Err  error
}

// SetupPlayReq is a setup/play request.
type SetupPlayReq struct {
	Client   *Client
	PathName string
	TrackID  int
	Req      *base.Request
	Res      chan SetupPlayRes
}

// RemoveReq is a remove request.
type RemoveReq struct {
	Client *Client
	Res    chan struct{}
}

// PlayReq is a play request.
type PlayReq struct {
	Client *Client
	Res    chan struct{}
}

// RecordReq is a record request.
type RecordReq struct {
	Client *Client
	Res    chan struct{}
}

// PauseReq is a pause request.
type PauseReq struct {
	Client *Client
	Res    chan struct{}
}

// Path is implemented by path.Path.
type Path interface {
	Name() string
	SourceTrackCount() int
	Conf() *conf.PathConf
	OnClientRemove(RemoveReq)
	OnClientPlay(PlayReq)
	OnClientRecord(RecordReq)
	OnClientPause(PauseReq)
	OnFrame(int, gortsplib.StreamType, []byte)
}

// Parent is implemented by clientman.ClientMan.
type Parent interface {
	Log(logger.Level, string, ...interface{})
	OnClientClose(*Client)
	OnClientDescribe(DescribeReq)
	OnClientAnnounce(AnnounceReq)
	OnClientSetupPlay(SetupPlayReq)
}

// Client is a RTSP
type Client struct {
	rtspPort            int
	readTimeout         time.Duration
	runOnConnect        string
	runOnConnectRestart bool
	protocols           map[gortsplib.StreamProtocol]struct{}
	wg                  *sync.WaitGroup
	stats               *stats.Stats
	conn                *gortsplib.ServerConn
	parent              Parent

	path          Path
	authUser      string
	authPass      string
	authValidator *auth.Validator
	authFailures  int
	onReadCmd     *externalcmd.Cmd
	onPublishCmd  *externalcmd.Cmd

	// in
	terminate chan struct{}
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
		conn:                conn,
		parent:              parent,
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
		reqPath, ok := req.URL.RTSPPath()
		if !ok {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, fmt.Errorf("invalid path (%s)", req.URL)
		}

		resc := make(chan DescribeRes)
		c.parent.OnClientDescribe(DescribeReq{c, reqPath, req, resc})
		res := <-resc

		if res.Err != nil {
			switch terr := res.Err.(type) {
			case errAuthNotCritical:
				return terr.Response, nil

			case errAuthCritical:
				// wait some seconds to stop brute force attacks
				select {
				case <-time.After(pauseAfterAuthError):
				case <-c.terminate:
				}
				return terr.Response, errTerminated

			case ErrNoOnePublishing:
				return &base.Response{
					StatusCode: base.StatusNotFound,
				}, res.Err

			default:
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, res.Err
			}
		}

		if res.Redirect != "" {
			return &base.Response{
				StatusCode: base.StatusMovedPermanently,
				Header: base.Header{
					"Location": base.HeaderValue{res.Redirect},
				},
			}, nil
		}

		return &base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"Content-Base": base.HeaderValue{req.URL.String() + "/"},
				"Content-Type": base.HeaderValue{"application/sdp"},
			},
			Body: res.SDP,
		}, nil
	}

	onAnnounce := func(req *base.Request, tracks gortsplib.Tracks) (*base.Response, error) {
		reqPath, ok := req.URL.RTSPPath()
		if !ok {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, fmt.Errorf("invalid path (%s)", req.URL)
		}

		resc := make(chan AnnounceRes)
		c.parent.OnClientAnnounce(AnnounceReq{c, reqPath, tracks, req, resc})
		res := <-resc

		if res.Err != nil {
			switch terr := res.Err.(type) {
			case errAuthNotCritical:
				return terr.Response, nil

			case errAuthCritical:
				// wait some seconds to stop brute force attacks
				select {
				case <-time.After(pauseAfterAuthError):
				case <-c.terminate:
				}
				return terr.Response, errTerminated

			default:
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, res.Err
			}
		}

		c.path = res.Path

		return &base.Response{
			StatusCode: base.StatusOK,
		}, nil
	}

	onSetup := func(req *base.Request, th *headers.Transport, trackID int) (*base.Response, error) {
		if th.Protocol == gortsplib.StreamProtocolUDP {
			if _, ok := c.protocols[gortsplib.StreamProtocolUDP]; !ok {
				return &base.Response{
					StatusCode: base.StatusUnsupportedTransport,
				}, nil
			}
		} else {
			if _, ok := c.protocols[gortsplib.StreamProtocolTCP]; !ok {
				return &base.Response{
					StatusCode: base.StatusUnsupportedTransport,
				}, nil
			}
		}

		switch c.conn.State() {
		case gortsplib.ServerConnStateInitial, gortsplib.ServerConnStatePrePlay: // play
			pathAndQuery, ok := req.URL.RTSPPathAndQuery()
			if !ok {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("invalid path (%s)", req.URL)
			}

			_, pathAndQuery, ok = base.PathSplitControlAttribute(pathAndQuery)
			if !ok {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("invalid path (%s)", req.URL)
			}

			reqPath, _ := base.PathSplitQuery(pathAndQuery)

			if c.path != nil && reqPath != c.path.Name() {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.Name(), reqPath)
			}

			resc := make(chan SetupPlayRes)
			c.parent.OnClientSetupPlay(SetupPlayReq{c, reqPath, trackID, req, resc})
			res := <-resc

			if res.Err != nil {
				switch terr := res.Err.(type) {
				case errAuthNotCritical:
					return terr.Response, nil

				case errAuthCritical:
					// wait some seconds to stop brute force attacks
					select {
					case <-time.After(pauseAfterAuthError):
					case <-c.terminate:
					}
					return terr.Response, errTerminated

				case ErrNoOnePublishing:
					return &base.Response{
						StatusCode: base.StatusNotFound,
					}, res.Err

				default:
					return &base.Response{
						StatusCode: base.StatusBadRequest,
					}, res.Err
				}
			}

			c.path = res.Path

		default: // record
			reqPathAndQuery, ok := req.URL.RTSPPathAndQuery()
			if !ok {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("invalid path (%s)", req.URL)
			}

			if !strings.HasPrefix(reqPathAndQuery, c.path.Name()) {
				return &base.Response{
						StatusCode: base.StatusBadRequest,
					}, fmt.Errorf("invalid path: must begin with '%s', but is '%s'",
						c.path.Name(), reqPathAndQuery)
			}
		}

		return &base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"Session": base.HeaderValue{sessionID},
			},
		}, nil
	}

	onPlay := func(req *base.Request) (*base.Response, error) {
		if c.conn.State() == gortsplib.ServerConnStatePrePlay {
			reqPath, ok := req.URL.RTSPPath()
			if !ok {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("invalid path (%s)", req.URL)
			}

			// path can end with a slash, remove it
			reqPath = strings.TrimSuffix(reqPath, "/")

			if reqPath != c.path.Name() {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.Name(), reqPath)
			}

			c.startPlay()
		}

		return &base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"Session": base.HeaderValue{sessionID},
			},
		}, nil
	}

	onRecord := func(req *base.Request) (*base.Response, error) {
		reqPath, ok := req.URL.RTSPPath()
		if !ok {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, fmt.Errorf("invalid path (%s)", req.URL)
		}

		// path can end with a slash, remove it
		reqPath = strings.TrimSuffix(reqPath, "/")

		if reqPath != c.path.Name() {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.Name(), reqPath)
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
		switch c.conn.State() {
		case gortsplib.ServerConnStatePlay:
			c.stopPlay()
			res := make(chan struct{})
			c.path.OnClientPause(PauseReq{c, res})
			<-res

		case gortsplib.ServerConnStateRecord:
			c.stopRecord()
			res := make(chan struct{})
			c.path.OnClientPause(PauseReq{c, res})
			<-res
		}

		return &base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"Session": base.HeaderValue{sessionID},
			},
		}, nil
	}

	onFrame := func(trackID int, streamType gortsplib.StreamType, payload []byte) {
		if c.conn.State() != gortsplib.ServerConnStateRecord {
			return
		}

		c.path.OnFrame(trackID, streamType, payload)
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

		switch c.conn.State() {
		case gortsplib.ServerConnStatePlay:
			c.stopPlay()

		case gortsplib.ServerConnStateRecord:
			c.stopRecord()
		}

		if c.path != nil {
			res := make(chan struct{})
			c.path.OnClientRemove(RemoveReq{c, res})
			<-res
			c.path = nil
		}

		c.parent.OnClientClose(c)
		<-c.terminate

	case <-c.terminate:
		c.conn.Close()
		<-readDone

		switch c.conn.State() {
		case gortsplib.ServerConnStatePlay:
			c.stopPlay()

		case gortsplib.ServerConnStateRecord:
			c.stopRecord()
		}

		if c.path != nil {
			res := make(chan struct{})
			c.path.OnClientRemove(RemoveReq{c, res})
			<-res
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
func (c *Client) Authenticate(authMethods []headers.AuthMethod, ips []interface{},
	user string, pass string, req *base.Request, altURL *base.URL) error {
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
		// reset authValidator every time the credentials change
		if c.authValidator == nil || c.authUser != user || c.authPass != pass {
			c.authUser = user
			c.authPass = pass
			c.authValidator = auth.NewValidator(user, pass, authMethods)
		}

		err := c.authValidator.ValidateHeader(req.Header["Authorization"],
			req.Method, req.URL, altURL)
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
						"WWW-Authenticate": c.authValidator.GenerateHeader(),
					},
				}}
			}

			if c.authFailures > 1 {
				c.log(logger.Debug, "WARN: unauthorized: %s", err)
			}

			return errAuthNotCritical{&base.Response{
				StatusCode: base.StatusUnauthorized,
				Header: base.Header{
					"WWW-Authenticate": c.authValidator.GenerateHeader(),
				},
			}}
		}
	}

	// login successful, reset authFailures
	c.authFailures = 0

	return nil
}

func (c *Client) startPlay() {
	res := make(chan struct{})
	c.path.OnClientPlay(PlayReq{c, res})
	<-res

	c.log(logger.Info, "is reading from path '%s', %d %s with %s", c.path.Name(),
		c.conn.SetuppedTracksLen(),
		func() string {
			if c.conn.SetuppedTracksLen() == 1 {
				return "track"
			}
			return "tracks"
		}(),
		*c.conn.SetuppedTracksProtocol())

	if c.path.Conf().RunOnRead != "" {
		c.onReadCmd = externalcmd.New(c.path.Conf().RunOnRead, c.path.Conf().RunOnReadRestart, externalcmd.Environment{
			Path: c.path.Name(),
			Port: strconv.FormatInt(int64(c.rtspPort), 10),
		})
	}
}

func (c *Client) stopPlay() {
	if c.path.Conf().RunOnRead != "" {
		c.onReadCmd.Close()
	}
}

func (c *Client) startRecord() {
	res := make(chan struct{})
	c.path.OnClientRecord(RecordReq{c, res})
	<-res

	c.log(logger.Info, "is publishing to path '%s', %d %s with %s", c.path.Name(),
		c.conn.SetuppedTracksLen(),
		func() string {
			if c.conn.SetuppedTracksLen() == 1 {
				return "track"
			}
			return "tracks"
		}(),
		*c.conn.SetuppedTracksProtocol())

	if c.path.Conf().RunOnPublish != "" {
		c.onPublishCmd = externalcmd.New(c.path.Conf().RunOnPublish, c.path.Conf().RunOnPublishRestart, externalcmd.Environment{
			Path: c.path.Name(),
			Port: strconv.FormatInt(int64(c.rtspPort), 10),
		})
	}
}

func (c *Client) stopRecord() {
	if c.path.Conf().RunOnPublish != "" {
		c.onPublishCmd.Close()
	}
}

// OnReaderFrame implements path.Reader.
func (c *Client) OnReaderFrame(trackID int, streamType base.StreamType, buf []byte) {
	if !c.conn.HasSetuppedTrack(trackID) {
		return
	}

	c.conn.WriteFrame(trackID, streamType, buf)
}
