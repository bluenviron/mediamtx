package core

import (
	"fmt"
	"net"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/auth"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/headers"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
)

const (
	rtspPauseAfterAuthError = 2 * time.Second
)

type rtspConnParent interface {
	logger.Writer
	getISTLS() bool
	getServer() *gortsplib.Server
}

type rtspConn struct {
	*conn

	isTLS       bool
	rtspAddress string
	authMethods []headers.AuthMethod
	readTimeout conf.StringDuration
	pathManager *pathManager
	rconn       *gortsplib.ServerConn
	parent      rtspConnParent

	uuid         uuid.UUID
	created      time.Time
	authNonce    string
	authFailures int
}

func newRTSPConn(
	isTLS bool,
	rtspAddress string,
	authMethods []headers.AuthMethod,
	readTimeout conf.StringDuration,
	runOnConnect string,
	runOnConnectRestart bool,
	runOnDisconnect string,
	externalCmdPool *externalcmd.Pool,
	pathManager *pathManager,
	conn *gortsplib.ServerConn,
	parent rtspConnParent,
) *rtspConn {
	c := &rtspConn{
		isTLS:       isTLS,
		rtspAddress: rtspAddress,
		authMethods: authMethods,
		readTimeout: readTimeout,
		pathManager: pathManager,
		rconn:       conn,
		parent:      parent,
		uuid:        uuid.New(),
		created:     time.Now(),
	}

	c.conn = newConn(
		rtspAddress,
		runOnConnect,
		runOnConnectRestart,
		runOnDisconnect,
		externalCmdPool,
		c,
	)

	c.Log(logger.Info, "opened")

	c.conn.open(defs.APIPathSourceOrReader{
		Type: func() string {
			if isTLS {
				return "rtspsConn"
			}
			return "rtspConn"
		}(),
		ID: c.uuid.String(),
	})

	return c
}

func (c *rtspConn) Log(level logger.Level, format string, args ...interface{}) {
	c.parent.Log(level, "[conn %v] "+format, append([]interface{}{c.rconn.NetConn().RemoteAddr()}, args...)...)
}

// Conn returns the RTSP connection.
func (c *rtspConn) Conn() *gortsplib.ServerConn {
	return c.rconn
}

func (c *rtspConn) remoteAddr() net.Addr {
	return c.rconn.NetConn().RemoteAddr()
}

func (c *rtspConn) ip() net.IP {
	return c.rconn.NetConn().RemoteAddr().(*net.TCPAddr).IP
}

// onClose is called by rtspServer.
func (c *rtspConn) onClose(err error) {
	c.Log(logger.Info, "closed: %v", err)

	c.conn.close()
}

// onRequest is called by rtspServer.
func (c *rtspConn) onRequest(req *base.Request) {
	c.Log(logger.Debug, "[c->s] %v", req)
}

// OnResponse is called by rtspServer.
func (c *rtspConn) OnResponse(res *base.Response) {
	c.Log(logger.Debug, "[s->c] %v", res)
}

// onDescribe is called by rtspServer.
func (c *rtspConn) onDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	if len(ctx.Path) == 0 || ctx.Path[0] != '/' {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, nil, fmt.Errorf("invalid path")
	}
	ctx.Path = ctx.Path[1:]

	if c.authNonce == "" {
		var err error
		c.authNonce, err = auth.GenerateNonce()
		if err != nil {
			return &base.Response{
				StatusCode: base.StatusInternalServerError,
			}, nil, err
		}
	}

	res := c.pathManager.describe(pathDescribeReq{
		accessRequest: pathAccessRequest{
			name:        ctx.Path,
			query:       ctx.Query,
			ip:          c.ip(),
			proto:       authProtocolRTSP,
			id:          &c.uuid,
			rtspRequest: ctx.Request,
			rtspNonce:   c.authNonce,
		},
	})

	if res.err != nil {
		switch terr := res.err.(type) {
		case *errAuthentication:
			res, err := c.handleAuthError(terr)
			return res, nil, err

		case errPathNoOnePublishing:
			return &base.Response{
				StatusCode: base.StatusNotFound,
			}, nil, res.err

		default:
			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, nil, res.err
		}
	}

	if res.redirect != "" {
		return &base.Response{
			StatusCode: base.StatusMovedPermanently,
			Header: base.Header{
				"Location": base.HeaderValue{res.redirect},
			},
		}, nil, nil
	}

	var stream *gortsplib.ServerStream
	if !c.parent.getISTLS() {
		stream = res.stream.RTSPStream(c.parent.getServer())
	} else {
		stream = res.stream.RTSPSStream(c.parent.getServer())
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, stream, nil
}

func (c *rtspConn) handleAuthError(authErr error) (*base.Response, error) {
	c.authFailures++

	// VLC with login prompt sends 4 requests:
	// 1) without credentials
	// 2) with password but without username
	// 3) without credentials
	// 4) with password and username
	// therefore we must allow up to 3 failures
	if c.authFailures <= 3 {
		return &base.Response{
			StatusCode: base.StatusUnauthorized,
			Header: base.Header{
				"WWW-Authenticate": auth.GenerateWWWAuthenticate(c.authMethods, "IPCAM", c.authNonce),
			},
		}, nil
	}

	// wait some seconds to stop brute force attacks
	<-time.After(rtspPauseAfterAuthError)

	return &base.Response{
		StatusCode: base.StatusUnauthorized,
	}, authErr
}

func (c *rtspConn) apiItem() *defs.APIRTSPConn {
	return &defs.APIRTSPConn{
		ID:            c.uuid,
		Created:       c.created,
		RemoteAddr:    c.remoteAddr().String(),
		BytesReceived: c.rconn.BytesReceived(),
		BytesSent:     c.rconn.BytesSent(),
	}
}
