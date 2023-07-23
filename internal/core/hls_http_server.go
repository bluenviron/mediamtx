package core

import (
	_ "embed"
	"fmt"
	"net"
	"net/http"
	gopath "path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
)

const (
	hlsPauseAfterAuthError = 2 * time.Second
)

//go:embed hls_index.html
var hlsIndex []byte

type hlsHTTPServerParent interface {
	logger.Writer
	handleRequest(req hlsMuxerHandleRequestReq)
}

type hlsHTTPServer struct {
	allowOrigin string
	pathManager *pathManager
	parent      hlsHTTPServerParent

	inner *httpServer
}

func newHLSHTTPServer( //nolint:dupl
	address string,
	encryption bool,
	serverKey string,
	serverCert string,
	allowOrigin string,
	trustedProxies conf.IPsOrCIDRs,
	readTimeout conf.StringDuration,
	pathManager *pathManager,
	parent hlsHTTPServerParent,
) (*hlsHTTPServer, error) {
	if encryption {
		if serverCert == "" {
			return nil, fmt.Errorf("server cert is missing")
		}
	} else {
		serverKey = ""
		serverCert = ""
	}

	s := &hlsHTTPServer{
		allowOrigin: allowOrigin,
		pathManager: pathManager,
		parent:      parent,
	}

	router := gin.New()
	httpSetTrustedProxies(router, trustedProxies)

	router.NoRoute(httpLoggerMiddleware(s), httpServerHeaderMiddleware, s.onRequest)

	var err error
	s.inner, err = newHTTPServer(
		address,
		readTimeout,
		serverCert,
		serverKey,
		router,
	)
	if err != nil {
		return nil, err
	}

	return s, nil
}

func (s *hlsHTTPServer) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, format, args...)
}

func (s *hlsHTTPServer) close() {
	s.inner.close()
}

func (s *hlsHTTPServer) onRequest(ctx *gin.Context) {
	ctx.Writer.Header().Set("Access-Control-Allow-Origin", s.allowOrigin)
	ctx.Writer.Header().Set("Access-Control-Allow-Credentials", "true")

	switch ctx.Request.Method {
	case http.MethodOptions:
		ctx.Writer.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET")
		ctx.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Range")
		ctx.Writer.WriteHeader(http.StatusOK)
		return

	case http.MethodGet:

	default:
		return
	}

	// remove leading prefix
	pa := ctx.Request.URL.Path[1:]

	var dir string
	var fname string

	switch {
	case pa == "", pa == "favicon.ico":
		return

	case strings.HasSuffix(pa, ".m3u8") ||
		strings.HasSuffix(pa, ".ts") ||
		strings.HasSuffix(pa, ".mp4") ||
		strings.HasSuffix(pa, ".mp"):
		dir, fname = gopath.Dir(pa), gopath.Base(pa)

		if strings.HasSuffix(fname, ".mp") {
			fname += "4"
		}

	default:
		dir, fname = pa, ""

		if !strings.HasSuffix(dir, "/") {
			l := "/" + dir + "/"
			if ctx.Request.URL.RawQuery != "" {
				l += "?" + ctx.Request.URL.RawQuery
			}
			ctx.Writer.Header().Set("Location", l)
			ctx.Writer.WriteHeader(http.StatusMovedPermanently)
			return
		}
	}

	dir = strings.TrimSuffix(dir, "/")
	if dir == "" {
		return
	}

	user, pass, hasCredentials := ctx.Request.BasicAuth()

	res := s.pathManager.getConfForPath(pathGetConfForPathReq{
		name:    dir,
		publish: false,
		credentials: authCredentials{
			query: ctx.Request.URL.RawQuery,
			ip:    net.ParseIP(ctx.ClientIP()),
			user:  user,
			pass:  pass,
			proto: authProtocolWebRTC,
		},
	})
	if res.err != nil {
		if terr, ok := res.err.(*errAuthentication); ok {
			if !hasCredentials {
				ctx.Header("WWW-Authenticate", `Basic realm="mediamtx"`)
				ctx.Writer.WriteHeader(http.StatusUnauthorized)
				return
			}

			ip := ctx.ClientIP()
			_, port, _ := net.SplitHostPort(ctx.Request.RemoteAddr)
			remoteAddr := net.JoinHostPort(ip, port)

			s.Log(logger.Info, "connection %v failed to authenticate: %v", remoteAddr, terr.message)

			// wait some seconds to stop brute force attacks
			<-time.After(hlsPauseAfterAuthError)

			ctx.Writer.WriteHeader(http.StatusUnauthorized)
			return
		}

		ctx.Writer.WriteHeader(http.StatusNotFound)
		return
	}

	switch fname {
	case "":
		ctx.Writer.Header().Set("Content-Type", "text/html")
		ctx.Writer.WriteHeader(http.StatusOK)
		ctx.Writer.Write(hlsIndex)

	default:
		s.parent.handleRequest(hlsMuxerHandleRequestReq{
			path: dir,
			file: fname,
			ctx:  ctx,
		})
	}
}
