package core

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
)

const (
	rtspConnPauseAfterAuthError = 2 * time.Second
)

func isTeardownErr(err error) bool {
	_, ok := err.(liberrors.ErrServerSessionTeardown)
	return ok
}

func isTerminatedErr(err error) bool {
	_, ok := err.(liberrors.ErrServerTerminated)
	return ok
}

type rtspConnPathMan interface {
	OnReadPublisherDescribe(readPublisherDescribeReq)
}

type rtspConnParent interface {
	Log(logger.Level, string, ...interface{})
}

type rtspConn struct {
	rtspAddress         string
	readTimeout         time.Duration
	runOnConnect        string
	runOnConnectRestart bool
	pathMan             rtspConnPathMan
	stats               *stats
	conn                *gortsplib.ServerConn
	parent              rtspConnParent

	onConnectCmd  *externalcmd.Cmd
	authUser      string
	authPass      string
	authValidator *auth.Validator
	authFailures  int
}

func newRTSPConn(
	rtspAddress string,
	readTimeout time.Duration,
	runOnConnect string,
	runOnConnectRestart bool,
	pathMan rtspConnPathMan,
	stats *stats,
	conn *gortsplib.ServerConn,
	parent rtspConnParent) *rtspConn {
	c := &rtspConn{
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
func (c *rtspConn) ParentClose(err error) {
	if err != io.EOF && !isTeardownErr(err) && !isTerminatedErr(err) {
		c.log(logger.Info, "ERR: %v", err)
	}

	c.log(logger.Info, "closed")

	if c.onConnectCmd != nil {
		c.onConnectCmd.Close()
	}
}

func (c *rtspConn) log(level logger.Level, format string, args ...interface{}) {
	c.parent.Log(level, "[conn %v] "+format, append([]interface{}{c.conn.NetConn().RemoteAddr()}, args...)...)
}

// Conn returns the RTSP connection.
func (c *rtspConn) Conn() *gortsplib.ServerConn {
	return c.conn
}

func (c *rtspConn) ip() net.IP {
	return c.conn.NetConn().RemoteAddr().(*net.TCPAddr).IP
}

// OnRequest is called by rtspserver.Server.
func (c *rtspConn) OnRequest(req *base.Request) {
	c.log(logger.Debug, "[c->s] %v", req)
}

// OnResponse is called by rtspserver.Server.
func (c *rtspConn) OnResponse(res *base.Response) {
	c.log(logger.Debug, "[s->c] %v", res)
}

// OnDescribe is called by rtspserver.Server.
func (c *rtspConn) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
	resc := make(chan readPublisherDescribeRes)
	c.pathMan.OnReadPublisherDescribe(readPublisherDescribeReq{
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
		case readPublisherErrAuthNotCritical:
			return terr.Response, nil, nil

		case readPublisherErrAuthCritical:
			// wait some seconds to stop brute force attacks
			<-time.After(rtspConnPauseAfterAuthError)

			return terr.Response, nil, errors.New(terr.Message)

		case readPublisherErrNoOnePublishing:
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
	}, res.Stream, nil
}

// ValidateCredentials allows to validate the credentials of a path.
func (c *rtspConn) ValidateCredentials(
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

	err := c.authValidator.ValidateRequest(req, altURL)
	if err != nil {
		c.authFailures++

		// vlc with login prompt sends 4 requests:
		// 1) without credentials
		// 2) with password but without username
		// 3) without credentials
		// 4) with password and username
		// therefore we must allow up to 3 failures
		if c.authFailures > 3 {
			return readPublisherErrAuthCritical{
				Message: "unauthorized: " + err.Error(),
				Response: &base.Response{
					StatusCode: base.StatusUnauthorized,
				},
			}
		}

		if c.authFailures > 1 {
			c.log(logger.Debug, "WARN: unauthorized: %s", err)
		}

		return readPublisherErrAuthNotCritical{
			Response: &base.Response{
				StatusCode: base.StatusUnauthorized,
				Header: base.Header{
					"WWW-Authenticate": c.authValidator.Header(),
				},
			},
		}
	}

	// login successful, reset authFailures
	c.authFailures = 0

	return nil
}
