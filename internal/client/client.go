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

type describeData struct {
	sdp      []byte
	redirect string
	err      error
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
	describeData chan describeData // from path
	terminate    chan struct{}
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
				Body: res.sdp,
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

		c.path = path

		return &base.Response{
			StatusCode: base.StatusOK,
		}, nil
	}

	onSetup := func(req *base.Request, th *headers.Transport, trackID int) (*base.Response, error) {
		basePath, _, ok := req.URL.BasePathControlAttr()
		if !ok {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, fmt.Errorf("unable to find control attribute (%s)", req.URL)
		}

		switch c.conn.State() {
		case gortsplib.ServerConnStateInitial, gortsplib.ServerConnStatePrePlay: // play
			if c.path != nil && basePath != c.path.Name() {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.Name(), basePath)
			}

			// play with UDP
			if th.Protocol == gortsplib.StreamProtocolUDP {
				if _, ok := c.protocols[gortsplib.StreamProtocolUDP]; !ok {
					return &base.Response{
						StatusCode: base.StatusUnsupportedTransport,
					}, nil
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

				return &base.Response{
					StatusCode: base.StatusOK,
					Header: base.Header{
						"Session": base.HeaderValue{sessionID},
					},
				}, nil
			}

			// play with TCP

			if _, ok := c.protocols[gortsplib.StreamProtocolTCP]; !ok {
				return &base.Response{
					StatusCode: base.StatusUnsupportedTransport,
				}, nil
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

			return &base.Response{
				StatusCode: base.StatusOK,
				Header: base.Header{
					"Session": base.HeaderValue{sessionID},
				},
			}, nil

		default: // record
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

				if th.ClientPorts == nil {
					return &base.Response{
						StatusCode: base.StatusBadRequest,
					}, fmt.Errorf("transport header does not have valid client ports (%s)", req.Header["Transport"])
				}

				if c.conn.TracksLen() >= c.path.SourceTrackCount() {
					return &base.Response{
						StatusCode: base.StatusBadRequest,
					}, fmt.Errorf("all the tracks have already been setup")
				}

				return &base.Response{
					StatusCode: base.StatusOK,
					Header: base.Header{
						"Session": base.HeaderValue{sessionID},
					},
				}, nil
			}

			// record with TCP

			if _, ok := c.protocols[gortsplib.StreamProtocolTCP]; !ok {
				return &base.Response{
					StatusCode: base.StatusUnsupportedTransport,
				}, nil
			}

			if c.conn.TracksLen() >= c.path.SourceTrackCount() {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("all the tracks have already been setup")
			}

			return &base.Response{
				StatusCode: base.StatusOK,
				Header: base.Header{
					"Session": base.HeaderValue{sessionID},
				},
			}, nil
		}
	}

	onPlay := func(req *base.Request) (*base.Response, error) {
		if c.conn.State() == gortsplib.ServerConnStatePrePlay {
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
			c.path.OnClientPause(c)

		case gortsplib.ServerConnStateRecord:
			c.stopRecord()
			c.path.OnClientPause(c)
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
			c.path.OnClientRemove(c)
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
		// reset authValidator every time the credentials change
		if c.authValidator == nil || c.authUser != user || c.authPass != pass {
			c.authUser = user
			c.authPass = pass
			c.authValidator = auth.NewValidator(user, pass, authMethods)
		}

		err := c.authValidator.ValidateHeader(req.Header["Authorization"], req.Method, req.URL)
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
	c.path.OnClientPlay(c)

	c.log(logger.Info, "is reading from path '%s', %d %s with %s", c.path.Name(), c.conn.TracksLen(), func() string {
		if c.conn.TracksLen() == 1 {
			return "track"
		}
		return "tracks"
	}(), *c.conn.TracksProtocol())

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
	c.path.OnClientRecord(c)

	c.log(logger.Info, "is publishing to path '%s', %d %s with %s", c.path.Name(), c.conn.TracksLen(), func() string {
		if c.conn.TracksLen() == 1 {
			return "track"
		}
		return "tracks"
	}(), *c.conn.TracksProtocol())

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
	if !c.conn.HasTrack(trackID) {
		return
	}

	c.conn.WriteFrame(trackID, streamType, buf)
}

// OnPathDescribeData is called by path.Path.
func (c *Client) OnPathDescribeData(sdp []byte, redirect string, err error) {
	c.describeData <- describeData{sdp, redirect, err}
}
