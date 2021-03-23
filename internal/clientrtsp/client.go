package clientrtsp

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/auth"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/headers"
	"github.com/aler9/gortsplib/pkg/liberrors"

	"github.com/aler9/rtsp-simple-server/internal/client"
	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/stats"
	"github.com/aler9/rtsp-simple-server/internal/streamproc"
)

const (
	sessionID           = "12345678"
	pauseAfterAuthError = 2 * time.Second
)

func ipEqualOrInRange(ip net.IP, ips []interface{}) bool {
	for _, item := range ips {
		switch titem := item.(type) {
		case net.IP:
			if titem.Equal(ip) {
				return true
			}

		case *net.IPNet:
			if titem.Contains(ip) {
				return true
			}
		}
	}
	return false
}

// Parent is implemented by clientman.ClientMan.
type Parent interface {
	Log(logger.Level, string, ...interface{})
	OnClientClose(client.Client)
	OnClientDescribe(client.DescribeReq)
	OnClientAnnounce(client.AnnounceReq)
	OnClientSetupPlay(client.SetupPlayReq)
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

	path          client.Path
	authUser      string
	authPass      string
	authValidator *auth.Validator
	authFailures  int

	// read only
	setuppedTracks map[int]*gortsplib.Track
	onReadCmd      *externalcmd.Cmd

	// publish only
	sp           *streamproc.StreamProc
	onPublishCmd *externalcmd.Cmd

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
			return "RTSP/TLS"
		}
		return "RTSP/TCP"
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

// IsClient implements client.Client.
func (c *Client) IsClient() {}

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

	onDescribe := func(ctx *gortsplib.ServerConnDescribeCtx) (*base.Response, []byte, error) {
		resc := make(chan client.DescribeRes)
		c.parent.OnClientDescribe(client.DescribeReq{c, ctx.Path, ctx.Req, resc}) //nolint:govet
		res := <-resc

		if res.Err != nil {
			switch terr := res.Err.(type) {
			case client.ErrAuthNotCritical:
				return terr.Response, nil, nil

			case client.ErrAuthCritical:
				// wait some seconds to stop brute force attacks
				select {
				case <-time.After(pauseAfterAuthError):
				case <-c.terminate:
				}
				return terr.Response, nil, errTerminated

			case client.ErrNoOnePublishing:
				return &base.Response{
					StatusCode: base.StatusNotFound,
				}, nil, res.Err

			default:
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, nil, res.Err
			}
		}

		if res.Redirect != "" {
			return &base.Response{
				StatusCode: base.StatusMovedPermanently,
				Header: base.Header{
					"Location": base.HeaderValue{res.Redirect},
				},
			}, nil, nil
		}

		return &base.Response{
			StatusCode: base.StatusOK,
		}, res.SDP, nil
	}

	onAnnounce := func(ctx *gortsplib.ServerConnAnnounceCtx) (*base.Response, error) {
		resc := make(chan client.AnnounceRes)
		c.parent.OnClientAnnounce(client.AnnounceReq{c, ctx.Path, ctx.Tracks, ctx.Req, resc}) //nolint:govet
		res := <-resc

		if res.Err != nil {
			switch terr := res.Err.(type) {
			case client.ErrAuthNotCritical:
				return terr.Response, nil

			case client.ErrAuthCritical:
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

	onSetup := func(ctx *gortsplib.ServerConnSetupCtx) (*base.Response, error) {
		if ctx.Transport.Protocol == gortsplib.StreamProtocolUDP {
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
			resc := make(chan client.SetupPlayRes)
			c.parent.OnClientSetupPlay(client.SetupPlayReq{c, ctx.Path, ctx.Req, resc}) //nolint:govet
			res := <-resc

			if res.Err != nil {
				switch terr := res.Err.(type) {
				case client.ErrAuthNotCritical:
					return terr.Response, nil

				case client.ErrAuthCritical:
					// wait some seconds to stop brute force attacks
					select {
					case <-time.After(pauseAfterAuthError):
					case <-c.terminate:
					}
					return terr.Response, errTerminated

				case client.ErrNoOnePublishing:
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

			if ctx.TrackID >= len(res.Tracks) {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("track %d does not exist", ctx.TrackID)
			}

			if c.setuppedTracks == nil {
				c.setuppedTracks = make(map[int]*gortsplib.Track)
			}
			c.setuppedTracks[ctx.TrackID] = res.Tracks[ctx.TrackID]
		}

		return &base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"Session": base.HeaderValue{sessionID},
			},
		}, nil
	}

	onPlay := func(ctx *gortsplib.ServerConnPlayCtx) (*base.Response, error) {
		h := base.Header{
			"Session": base.HeaderValue{sessionID},
		}

		if c.conn.State() == gortsplib.ServerConnStatePrePlay {
			if ctx.Path != c.path.Name() {
				return &base.Response{
					StatusCode: base.StatusBadRequest,
				}, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.Name(), ctx.Path)
			}

			res := c.playStart()

			// add RTP-Info
			var ri headers.RTPInfo
			for trackID, ti := range res.TrackInfos {
				if !ti.Initialized {
					continue
				}

				track, ok := c.setuppedTracks[trackID]
				if !ok {
					continue
				}

				u := &base.URL{
					Scheme: ctx.Req.URL.Scheme,
					User:   ctx.Req.URL.User,
					Host:   ctx.Req.URL.Host,
					Path:   "/" + c.path.Name(),
				}
				u.AddControlAttribute("trackID=" + strconv.FormatInt(int64(trackID), 10))

				clockRate, _ := track.ClockRate()
				ts := uint32(uint64(time.Since(ti.NTPTime).Seconds()*float64(clockRate)) +
					uint64(ti.RTPTime))
				lsn := ti.LastSequenceNumber

				ri = append(ri, &headers.RTPInfoEntry{
					URL:            u,
					SequenceNumber: &lsn,
					Timestamp:      &ts,
				})
			}
			if len(ri) > 0 {
				h["RTP-Info"] = ri.Write()
			}
		}

		return &base.Response{
			StatusCode: base.StatusOK,
			Header:     h,
		}, nil
	}

	onRecord := func(ctx *gortsplib.ServerConnRecordCtx) (*base.Response, error) {
		if ctx.Path != c.path.Name() {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, fmt.Errorf("path has changed, was '%s', now is '%s'", c.path.Name(), ctx.Path)
		}

		err := c.recordStart()
		if err != nil {
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, err
		}

		return &base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"Session": base.HeaderValue{sessionID},
			},
		}, nil
	}

	onPause := func(ctx *gortsplib.ServerConnPauseCtx) (*base.Response, error) {
		switch c.conn.State() {
		case gortsplib.ServerConnStatePlay:
			c.playStop()
			res := make(chan struct{})
			c.path.OnClientPause(client.PauseReq{c, res}) //nolint:govet
			<-res

		case gortsplib.ServerConnStateRecord:
			c.recordStop()
			res := make(chan struct{})
			c.path.OnClientPause(client.PauseReq{c, res}) //nolint:govet
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

		c.sp.OnFrame(trackID, streamType, payload)
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

		if err != io.EOF && err != errTerminated {
			if _, ok := err.(liberrors.ErrServerTeardown); !ok {
				c.log(logger.Info, "ERR: %s", err)
			}
		}

		switch c.conn.State() {
		case gortsplib.ServerConnStatePlay:
			c.playStop()

		case gortsplib.ServerConnStateRecord:
			c.recordStop()
		}

		if c.path != nil {
			res := make(chan struct{})
			c.path.OnClientRemove(client.RemoveReq{c, res}) //nolint:govet
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
			c.playStop()

		case gortsplib.ServerConnStateRecord:
			c.recordStop()
		}

		if c.path != nil {
			res := make(chan struct{})
			c.path.OnClientRemove(client.RemoveReq{c, res}) //nolint:govet
			<-res
			c.path = nil
		}
	}
}

// Authenticate performs an authentication.
func (c *Client) Authenticate(authMethods []headers.AuthMethod,
	pathName string, ips []interface{},
	user string, pass string, req interface{}) error {

	// validate ip
	if ips != nil {
		ip := c.ip()

		if !ipEqualOrInRange(ip, ips) {
			c.log(logger.Info, "ERR: ip '%s' not allowed", ip)

			return client.ErrAuthCritical{&base.Response{ //nolint:govet
				StatusCode: base.StatusUnauthorized,
			}}
		}
	}

	// validate user
	if user != "" {
		reqRTSP := req.(*base.Request)

		// reset authValidator every time the credentials change
		if c.authValidator == nil || c.authUser != user || c.authPass != pass {
			c.authUser = user
			c.authPass = pass
			c.authValidator = auth.NewValidator(user, pass, authMethods)
		}

		// VLC strips the control attribute
		// provide an alternative URL without the control attribute
		altURL := func() *base.URL {
			if reqRTSP.Method != base.Setup {
				return nil
			}
			return &base.URL{
				Scheme: reqRTSP.URL.Scheme,
				Host:   reqRTSP.URL.Host,
				Path:   "/" + pathName + "/",
			}
		}()

		err := c.authValidator.ValidateHeader(reqRTSP.Header["Authorization"],
			reqRTSP.Method, reqRTSP.URL, altURL)
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

				return client.ErrAuthCritical{&base.Response{ //nolint:govet
					StatusCode: base.StatusUnauthorized,
					Header: base.Header{
						"WWW-Authenticate": c.authValidator.GenerateHeader(),
					},
				}}
			}

			if c.authFailures > 1 {
				c.log(logger.Debug, "WARN: unauthorized: %s", err)
			}

			return client.ErrAuthNotCritical{&base.Response{ //nolint:govet
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

func (c *Client) playStart() client.PlayRes {
	resc := make(chan client.PlayRes)
	c.path.OnClientPlay(client.PlayReq{c, resc}) //nolint:govet
	res := <-resc

	tracksLen := len(c.conn.SetuppedTracks())

	c.log(logger.Info, "is reading from path '%s', %d %s with %s",
		c.path.Name(),
		tracksLen,
		func() string {
			if tracksLen == 1 {
				return "track"
			}
			return "tracks"
		}(),
		*c.conn.StreamProtocol())

	if c.path.Conf().RunOnRead != "" {
		c.onReadCmd = externalcmd.New(c.path.Conf().RunOnRead, c.path.Conf().RunOnReadRestart, externalcmd.Environment{
			Path: c.path.Name(),
			Port: strconv.FormatInt(int64(c.rtspPort), 10),
		})
	}

	return res
}

func (c *Client) playStop() {
	if c.path.Conf().RunOnRead != "" {
		c.onReadCmd.Close()
	}
}

func (c *Client) recordStart() error {
	resc := make(chan client.RecordRes)
	c.path.OnClientRecord(client.RecordReq{Client: c, Res: resc})
	res := <-resc

	if res.Err != nil {
		return res.Err
	}

	c.sp = res.SP

	tracksLen := len(c.conn.AnnouncedTracks())

	c.log(logger.Info, "is publishing to path '%s', %d %s with %s",
		c.path.Name(),
		tracksLen,
		func() string {
			if tracksLen == 1 {
				return "track"
			}
			return "tracks"
		}(),
		*c.conn.StreamProtocol())

	if c.path.Conf().RunOnPublish != "" {
		c.onPublishCmd = externalcmd.New(c.path.Conf().RunOnPublish, c.path.Conf().RunOnPublishRestart, externalcmd.Environment{
			Path: c.path.Name(),
			Port: strconv.FormatInt(int64(c.rtspPort), 10),
		})
	}

	return nil
}

func (c *Client) recordStop() {
	if c.path.Conf().RunOnPublish != "" {
		c.onPublishCmd.Close()
	}
}

// OnFrame implements path.Reader.
func (c *Client) OnFrame(trackID int, streamType gortsplib.StreamType, payload []byte) {
	if _, ok := c.conn.SetuppedTracks()[trackID]; !ok {
		return
	}

	c.conn.WriteFrame(trackID, streamType, payload)
}
