package rtsp

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	rtspauth "github.com/bluenviron/gortsplib/v5/pkg/auth"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/liberrors"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/hooks"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/rtsp"
)

func absoluteURL(req *base.Request, v string) string {
	if strings.HasPrefix(v, "/") {
		ur := base.URL{
			Scheme: req.URL.Scheme,
			Host:   req.URL.Host,
			Path:   v,
		}
		return ur.String()
	}

	return v
}

func tunnelLabel(t gortsplib.Tunnel) string {
	switch t {
	case gortsplib.TunnelHTTP:
		return "http"
	case gortsplib.TunnelWebSocket:
		return "websocket"
	case gortsplib.TunnelNone:
		return "none"
	}
	return "unknown"
}

type connParent interface {
	logger.Writer
	getSessionByRSessionUnsafe(rsession *gortsplib.ServerSession) *session
}

type conn struct {
	isTLS               bool
	rtspAddress         string
	authMethods         []rtspauth.VerifyMethod
	readTimeout         conf.Duration
	runOnConnect        string
	runOnConnectRestart bool
	runOnDisconnect     string
	externalCmdPool     *externalcmd.Pool
	pathManager         serverPathManager
	rconn               *gortsplib.ServerConn
	rserver             *gortsplib.Server
	parent              connParent

	uuid             uuid.UUID
	created          time.Time
	onDisconnectHook func()
}

func (c *conn) initialize() {
	c.uuid = uuid.New()
	c.created = time.Now()

	c.Log(logger.Info, "opened")

	c.onDisconnectHook = hooks.OnConnect(hooks.OnConnectParams{
		Logger:              c,
		ExternalCmdPool:     c.externalCmdPool,
		RunOnConnect:        c.runOnConnect,
		RunOnConnectRestart: c.runOnConnectRestart,
		RunOnDisconnect:     c.runOnDisconnect,
		RTSPAddress:         c.rtspAddress,
		Desc: defs.APIPathReader{
			Type: func() string {
				if c.isTLS {
					return "rtspsConn"
				}
				return "rtspConn"
			}(),
			ID: c.uuid.String(),
		},
	})
}

// Log implements logger.Writer.
func (c *conn) Log(level logger.Level, format string, args ...any) {
	c.parent.Log(level, "[conn %v] "+format, append([]any{c.rconn.NetConn().RemoteAddr()}, args...)...)
}

// Conn returns the RTSP connection.
func (c *conn) Conn() *gortsplib.ServerConn {
	return c.rconn
}

func (c *conn) remoteAddr() net.Addr {
	return c.rconn.NetConn().RemoteAddr()
}

func (c *conn) ip() net.IP {
	return c.rconn.NetConn().RemoteAddr().(*net.TCPAddr).IP
}

// onClose is called by rtspServer.
func (c *conn) onClose(err error) {
	c.Log(logger.Info, "closed: %v", err)

	c.onDisconnectHook()
}

// onRequest is called by rtspServer.
func (c *conn) onRequest(req *base.Request) {
	c.Log(logger.Debug, "[c->s] %v", req)
}

// OnResponse is called by rtspServer.
func (c *conn) OnResponse(res *base.Response) {
	c.Log(logger.Debug, "[s->c] %v", res)
}

// onDescribe is called by rtspServer.
func (c *conn) onDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	if len(ctx.Path) == 0 || ctx.Path[0] != '/' {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, nil, fmt.Errorf("invalid path")
	}
	ctx.Path = ctx.Path[1:]

	// CustomVerifyFunc prevents hashed credentials from working.
	// Use it only when strictly needed.
	var customVerifyFunc func(expectedUser, expectedPass string) bool
	if slices.Contains(c.authMethods, rtspauth.VerifyMethodDigestMD5) {
		customVerifyFunc = func(expectedUser, expectedPass string) bool {
			return c.rconn.VerifyCredentials(ctx.Request, expectedUser, expectedPass)
		}
	}

	res := c.pathManager.Describe(defs.PathDescribeReq{
		AccessRequest: defs.PathAccessRequest{
			Name:             ctx.Path,
			Query:            ctx.Query,
			Proto:            auth.ProtocolRTSP,
			ID:               &c.uuid,
			Credentials:      rtsp.Credentials(ctx.Request),
			IP:               c.ip(),
			CustomVerifyFunc: customVerifyFunc,
		},
	})

	if res.Err != nil {
		var terr *auth.Error
		if errors.As(res.Err, &terr) {
			res, err2 := c.handleAuthError(terr)
			return res, nil, err2
		}

		var terr2 defs.PathNoStreamAvailableError
		if errors.As(res.Err, &terr2) {
			return &base.Response{
				StatusCode: base.StatusNotFound,
			}, nil, res.Err
		}

		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, nil, res.Err
	}

	if res.Redirect != "" {
		return &base.Response{
			StatusCode: base.StatusMovedPermanently,
			Header: base.Header{
				"Location": base.HeaderValue{absoluteURL(ctx.Request, res.Redirect)},
			},
		}, nil, nil
	}

	var strm *gortsplib.ServerStream
	if !c.isTLS {
		strm = res.Stream.RTSPStream(c.rserver)
	} else {
		strm = res.Stream.RTSPSStream(c.rserver)
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, strm, nil
}

func (c *conn) handleAuthError(err *auth.Error) (*base.Response, error) {
	if err.AskCredentials {
		return &base.Response{
			StatusCode: base.StatusUnauthorized,
		}, liberrors.ErrServerAuth{}
	}

	// wait some seconds to delay brute force attacks
	<-time.After(auth.PauseAfterError)

	return &base.Response{
		StatusCode: base.StatusUnauthorized,
	}, err
}

func (c *conn) apiItem() *defs.APIRTSPConn {
	stats := c.rconn.Stats()

	return &defs.APIRTSPConn{
		ID:         c.uuid,
		Created:    c.created,
		RemoteAddr: c.remoteAddr().String(),
		Session: func() *uuid.UUID {
			sx := c.parent.getSessionByRSessionUnsafe(c.rconn.Session())
			if sx != nil {
				return &sx.uuid
			}
			return nil
		}(),
		Tunnel:        tunnelLabel(c.rconn.Transport().Tunnel),
		BytesReceived: stats.BytesReceived,
		BytesSent:     stats.BytesSent,
	}
}
