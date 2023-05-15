package core

import (
	"context"
	"crypto/tls"
	"log"
	"net"
	"net/http"
	gopath "path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/aler9/mediamtx/internal/conf"
	"github.com/aler9/mediamtx/internal/logger"
)

type hlsHTTPServerParent interface {
	logger.Writer
	handleRequest(req hlsMuxerHandleRequestReq)
}

type hlsHTTPServer struct {
	allowOrigin string
	parent      hlsHTTPServerParent

	ln    net.Listener
	inner *http.Server
}

func newHLSHTTPServer(
	address string,
	encryption bool,
	serverKey string,
	serverCert string,
	allowOrigin string,
	trustedProxies conf.IPsOrCIDRs,
	readTimeout conf.StringDuration,
	parent hlsHTTPServerParent,
) (*hlsHTTPServer, error) {
	ln, err := net.Listen(restrictNetwork("tcp", address))
	if err != nil {
		return nil, err
	}

	var tlsConfig *tls.Config
	if encryption {
		crt, err := tls.LoadX509KeyPair(serverCert, serverKey)
		if err != nil {
			ln.Close()
			return nil, err
		}

		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{crt},
		}
	}

	s := &hlsHTTPServer{
		allowOrigin: allowOrigin,
		parent:      parent,
		ln:          ln,
	}

	router := gin.New()
	httpSetTrustedProxies(router, trustedProxies)

	router.NoRoute(httpLoggerMiddleware(s), httpServerHeaderMiddleware, s.onRequest)

	s.inner = &http.Server{
		Handler:           router,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: time.Duration(readTimeout),
		ErrorLog:          log.New(&nilWriter{}, "", 0),
	}

	if tlsConfig != nil {
		go s.inner.ServeTLS(s.ln, "", "")
	} else {
		go s.inner.Serve(s.ln)
	}

	return s, nil
}

func (s *hlsHTTPServer) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, format, args...)
}

func (s *hlsHTTPServer) close() {
	s.inner.Shutdown(context.Background())
	s.ln.Close() // in case Shutdown() is called before Serve()
}

func (s *hlsHTTPServer) onRequest(ctx *gin.Context) {
	ctx.Writer.Header().Set("Access-Control-Allow-Origin", s.allowOrigin)
	ctx.Writer.Header().Set("Access-Control-Allow-Credentials", "true")

	switch ctx.Request.Method {
	case http.MethodGet:

	case http.MethodOptions:
		ctx.Writer.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		ctx.Writer.Header().Set("Access-Control-Allow-Headers", ctx.Request.Header.Get("Access-Control-Request-Headers"))
		ctx.Writer.WriteHeader(http.StatusOK)
		return

	default:
		return
	}

	// remove leading prefix
	pa := ctx.Request.URL.Path[1:]

	switch pa {
	case "", "favicon.ico":
		return
	}

	dir, fname := func() (string, string) {
		if strings.HasSuffix(pa, ".m3u8") ||
			strings.HasSuffix(pa, ".ts") ||
			strings.HasSuffix(pa, ".mp4") ||
			strings.HasSuffix(pa, ".mp") {
			return gopath.Dir(pa), gopath.Base(pa)
		}
		return pa, ""
	}()

	if fname == "" && !strings.HasSuffix(dir, "/") {
		ctx.Writer.Header().Set("Location", "/"+dir+"/")
		ctx.Writer.WriteHeader(http.StatusMovedPermanently)
		return
	}

	if strings.HasSuffix(fname, ".mp") {
		fname += "4"
	}

	dir = strings.TrimSuffix(dir, "/")

	s.parent.handleRequest(hlsMuxerHandleRequestReq{
		path: dir,
		file: fname,
		ctx:  ctx,
	})
}
