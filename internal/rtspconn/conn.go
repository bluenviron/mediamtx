package rtspconn

import (
	"errors"
	"io"
	"net"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/auth"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/headers"
	"github.com/aler9/gortsplib/pkg/liberrors"

	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/readpublisher"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

const (
	pauseAfterAuthError = 2 * time.Second
)

func isTeardownErr(err error) bool {
	_, ok := err.(liberrors.ErrServerSessionTeardown)
	return ok
}

func isTerminatedErr(err error) bool {
	_, ok := err.(liberrors.ErrServerTerminated)
	return ok
}

// PathMan is implemented by pathman.PathMan.
type PathMan interface {
	OnReadPublisherDescribe(readpublisher.DescribeReq)
}

// Parent is implemented by rtspserver.Server.
type Parent interface {
	Log(logger.Level, string, ...interface{})
}

// Conn is a RTSP server-side connection.
type Conn struct {
	rtspAddress         string
	readTimeout         time.Duration
	runOnConnect        string
	runOnConnectRestart bool
	pathMan             PathMan
	stats               *stats.Stats
	conn                *gortsplib.ServerConn
	parent              Parent

	onConnectCmd  *externalcmd.Cmd
	authUser      string
	authPass      string
	authValidator *auth.Validator
	authFailures  int
}

// New allocates a Conn.
func New(
	rtspAddress string,
	readTimeout time.Duration,
	runOnConnect string,
	runOnConnectRestart bool,
	pathMan PathMan,
	stats *stats.Stats,
	conn *gortsplib.ServerConn,
	parent Parent) *Conn {

	c := &Conn{
		rtspAddress:         rtspAddress,
		readTimeout:         readTimeout,
		runOnConnect:        runOnConnect,
		runOnConnectRestart: runOnConnectRestart,
		pathMan:             pathMan,
		stats:               stats,
		conn:                conn,
		parent:              parent,
	}

	c.log(logger.Info, "opened")

	if c.runOnConnect != "" {
		_, port, _ := net.SplitHostPort(c.rtspAddress)
		c.onConnectCmd = externalcmd.New(c.runOnConnect, c.runOnConnectRestart, externalcmd.Environment{
			Path: "",
			Port: port,
		})
	}

	return c
}

// ParentClose closes a Conn.
func (c *Conn) ParentClose(err error) {
	if err != io.EOF && !isTeardownErr(err) && !isTerminatedErr(err) {
		c.log(logger.Info, "ERR: %v", err)
	}

	c.log(logger.Info, "closed")

	if c.onConnectCmd != nil {
		c.onConnectCmd.Close()
	}
}

func (c *Conn) log(level logger.Level, format string, args ...interface{}) {
	c.parent.Log(level, "[conn %v] "+format, append([]interface{}{c.conn.NetConn().RemoteAddr()}, args...)...)
}

// Conn returns the RTSP connection.
func (c *Conn) Conn() *gortsplib.ServerConn {
	return c.conn
}

func (c *Conn) ip() net.IP {
	return c.conn.NetConn().RemoteAddr().(*net.TCPAddr).IP
}

// OnRequest is called by rtspserver.Server.
func (c *Conn) OnRequest(req *base.Request) {
	c.log(logger.Debug, "[c->s] %v", req)
}

// OnResponse is called by rtspserver.Server.
func (c *Conn) OnResponse(res *base.Response) {
	c.log(logger.Debug, "[s->c] %v", res)
}

// OnDescribe is called by rtspserver.Server.
func (c *Conn) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, []byte, error) {
	resc := make(chan readpublisher.DescribeRes)
	c.pathMan.OnReadPublisherDescribe(readpublisher.DescribeReq{
		PathName: ctx.Path,
		URL:      ctx.Req.URL,
		IP:       c.ip(),
		ValidateCredentials: func(authMethods []headers.AuthMethod, pathUser string, pathPass string) error {
			return c.ValidateCredentials(authMethods, pathUser, pathPass, ctx.Path, ctx.Req)
		},
		Res: resc,
	})
	res := <-resc

	if res.Err != nil {
		switch terr := res.Err.(type) {
		case readpublisher.ErrAuthNotCritical:
			return terr.Response, nil, nil

		case readpublisher.ErrAuthCritical:
			// wait some seconds to stop brute force attacks
			<-time.After(pauseAfterAuthError)

			return terr.Response, nil, errors.New(terr.Message)

		case readpublisher.ErrNoOnePublishing:
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

// ValidateCredentials allows to validate the credentials of a path.
func (c *Conn) ValidateCredentials(
	authMethods []headers.AuthMethod,
	pathUser string,
	pathPass string,
	pathName string,
	req *base.Request,
) error {

	// reset authValidator every time the credentials change
	if c.authValidator == nil || c.authUser != pathUser || c.authPass != pathPass {
		c.authUser = pathUser
		c.authPass = pathPass
		c.authValidator = auth.NewValidator(pathUser, pathPass, authMethods)
	}

	// VLC strips the control attribute
	// provide an alternative URL without the control attribute
	altURL := func() *base.URL {
		if req.Method != base.Setup {
			return nil
		}
		return &base.URL{
			Scheme: req.URL.Scheme,
			Host:   req.URL.Host,
			Path:   "/" + pathName + "/",
		}
	}()

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
			return readpublisher.ErrAuthCritical{
				Message: "unauthorized: " + err.Error(),
				Response: &base.Response{
					StatusCode: base.StatusUnauthorized,
					Header: base.Header{
						"WWW-Authenticate": c.authValidator.GenerateHeader(),
					},
				},
			}
		}

		if c.authFailures > 1 {
			c.log(logger.Debug, "WARN: unauthorized: %s", err)
		}

		return readpublisher.ErrAuthNotCritical{&base.Response{ //nolint:govet
			StatusCode: base.StatusUnauthorized,
			Header: base.Header{
				"WWW-Authenticate": c.authValidator.GenerateHeader(),
			},
		}}
	}

	// login successful, reset authFailures
	c.authFailures = 0

	return nil
}
