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

	"github.com/aler9/rtsp-simple-server/internal/conf"
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

type rtspConnParent interface {
	Log(logger.Level, string, ...interface{})
}

type rtspConn struct {
	rtspAddress         string
	authMethods         []headers.AuthMethod
	readTimeout         conf.StringDuration
	runOnConnect        string
	runOnConnectRestart bool
	pathManager         *pathManager
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
	authMethods []headers.AuthMethod,
	readTimeout conf.StringDuration,
	runOnConnect string,
	runOnConnectRestart bool,
	pathManager *pathManager,
	conn *gortsplib.ServerConn,
	parent rtspConnParent) *rtspConn {
	c := &rtspConn{
		rtspAddress:         rtspAddress,
		authMethods:         authMethods,
		readTimeout:         readTimeout,
		runOnConnect:        runOnConnect,
		runOnConnectRestart: runOnConnectRestart,
		pathManager:         pathManager,
		conn:                conn,
		parent:              parent,
	}

	c.log(logger.Info, "opened")

	if c.runOnConnect != "" {
		c.log(logger.Info, "runOnConnect command started")
		_, port, _ := net.SplitHostPort(c.rtspAddress)
		c.onConnectCmd = externalcmd.New(c.runOnConnect, c.runOnConnectRestart, externalcmd.Environment{
			Path: "",
			Port: port,
		})
	}

	return c
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

func (c *rtspConn) validateCredentials(
	pathUser conf.Credential,
	pathPass conf.Credential,
	pathName string,
	req *base.Request,
) error {
	// reset authValidator every time the credentials change
	if c.authValidator == nil || c.authUser != string(pathUser) || c.authPass != string(pathPass) {
		c.authUser = string(pathUser)
		c.authPass = string(pathPass)
		c.authValidator = auth.NewValidator(string(pathUser), string(pathPass), c.authMethods)
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
			return pathErrAuthCritical{
				Message: "unauthorized: " + err.Error(),
				Response: &base.Response{
					StatusCode: base.StatusUnauthorized,
				},
			}
		}

		if c.authFailures > 1 {
			c.log(logger.Debug, "WARN: unauthorized: %s", err)
		}

		return pathErrAuthNotCritical{
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

// OnClose is called by rtspServer.
func (c *rtspConn) OnClose(err error) {
	if err != io.EOF && !isTeardownErr(err) && !isTerminatedErr(err) {
		c.log(logger.Info, "ERR: %v", err)
	}

	c.log(logger.Info, "closed")

	if c.onConnectCmd != nil {
		c.onConnectCmd.Close()
		c.log(logger.Info, "runOnConnect command stopped")
	}
}

// OnRequest is called by rtspServer.
func (c *rtspConn) OnRequest(req *base.Request) {
	c.log(logger.Debug, "[c->s] %v", req)
}

// OnResponse is called by rtspServer.
func (c *rtspConn) OnResponse(res *base.Response) {
	c.log(logger.Debug, "[s->c] %v", res)
}

// OnDescribe is called by rtspServer.
func (c *rtspConn) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	res := c.pathManager.OnDescribe(pathDescribeReq{
		PathName: ctx.Path,
		URL:      ctx.Req.URL,
		IP:       c.ip(),
		ValidateCredentials: func(pathUser conf.Credential, pathPass conf.Credential) error {
			return c.validateCredentials(pathUser, pathPass, ctx.Path, ctx.Req)
		},
	})

	if res.Err != nil {
		switch terr := res.Err.(type) {
		case pathErrAuthNotCritical:
			return terr.Response, nil, nil

		case pathErrAuthCritical:
			// wait some seconds to stop brute force attacks
			<-time.After(rtspConnPauseAfterAuthError)

			return terr.Response, nil, errors.New(terr.Message)

		case pathErrNoOnePublishing:
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
	}, res.Stream.rtspStream, nil
}
