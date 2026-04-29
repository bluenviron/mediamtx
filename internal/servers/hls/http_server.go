package hls

import (
	_ "embed"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
)

//go:generate go run ./hlsjsdownloader

//go:embed index.html
var hlsIndex []byte

//go:embed hls.min.js
var hlsMinJS []byte

func trailingSlashLocation(rawPath string, rawQuery string) string {
	res := path.Clean(rawPath)
	res = strings.TrimLeft(res, "/\\")
	res = "/" + res + "/"

	if rawQuery != "" {
		res += "?" + rawQuery
	}

	return res
}

func sanitizeLocation(rawPath string, rawQuery string) string {
	res := path.Clean(rawPath)
	res = strings.TrimLeft(res, "/\\")
	res = "/" + res

	if rawQuery != "" {
		res += "?" + rawQuery
	}

	return res
}

func isIOS(userAgent string) bool {
	return strings.Contains(userAgent, "iPad") ||
		strings.Contains(userAgent, "iPhone") ||
		strings.Contains(userAgent, "iPod")
}

type httpServer struct {
	address        string
	dumpPackets    bool
	encryption     bool
	serverKey      string
	serverCert     string
	allowOrigins   []string
	trustedProxies conf.IPNetworks
	readTimeout    conf.Duration
	writeTimeout   conf.Duration
	pathManager    serverPathManager
	parent         *Server

	inner *httpp.Server
}

func (s *httpServer) initialize() error {
	router := gin.New()
	router.SetTrustedProxies(s.trustedProxies.ToTrustedProxies()) //nolint:errcheck

	router.Use(s.middlewarePreflightRequests)

	router.Use(s.onRequest)

	var proto string
	if s.encryption {
		proto = "hlss"
	} else {
		proto = "hls"
	}

	s.inner = &httpp.Server{
		Address:           s.address,
		AllowOrigins:      s.allowOrigins,
		DumpPackets:       s.dumpPackets,
		DumpPacketsPrefix: proto + "_server_conn",
		ReadTimeout:       time.Duration(s.readTimeout),
		WriteTimeout:      time.Duration(s.writeTimeout),
		Encryption:        s.encryption,
		ServerCert:        s.serverCert,
		ServerKey:         s.serverKey,
		Handler:           router,
		Parent:            s,
	}
	err := s.inner.Initialize()
	if err != nil {
		return err
	}

	return nil
}

// Log implements logger.Writer.
func (s *httpServer) Log(level logger.Level, format string, args ...any) {
	s.parent.Log(level, format, args...)
}

func (s *httpServer) close() {
	s.inner.Close()
}

func (s *httpServer) middlewarePreflightRequests(ctx *gin.Context) {
	if ctx.Request.Method == http.MethodOptions &&
		ctx.Request.Header.Get("Access-Control-Request-Method") != "" {
		ctx.Header("Access-Control-Allow-Methods", "OPTIONS, GET")
		ctx.Header("Access-Control-Allow-Headers", "Authorization, Range")
		ctx.AbortWithStatus(http.StatusNoContent)
		return
	}
}

func (s *httpServer) writeErrorNoLog(ctx *gin.Context, status int, err error) {
	ctx.AbortWithStatusJSON(status, &defs.APIError{
		Status: defs.APIErrorStatusError,
		Error:  err.Error(),
	})
}

func (s *httpServer) onRequest(ctx *gin.Context) {
	if ctx.Request.Method != http.MethodGet {
		return
	}

	// remove leading prefix
	pa := ctx.Request.URL.Path[1:]

	var dir string
	var fname string

	type contentType int

	const (
		index contentType = iota
		multivariantPlaylist
		mediaPlaylist
		segment
	)

	var contentTyp contentType

	switch {
	case strings.HasSuffix(pa, "/hls.min.js"):
		ctx.Header("Cache-Control", "max-age=3600")
		ctx.Header("Content-Type", "application/javascript")
		ctx.Writer.WriteHeader(http.StatusOK)
		ctx.Writer.Write(hlsMinJS)
		return

	case pa == "", pa == "favicon.ico", strings.HasSuffix(pa, "/hls.min.js.map"):
		return

	case strings.HasSuffix(pa, ".m3u8"):
		dir, fname = path.Dir(pa), path.Base(pa)

		if fname == "index.m3u8" {
			contentTyp = multivariantPlaylist
		} else {
			contentTyp = mediaPlaylist
		}

	case strings.HasSuffix(pa, ".ts") ||
		strings.HasSuffix(pa, ".mp4") ||
		strings.HasSuffix(pa, ".mp"):
		dir, fname = path.Dir(pa), path.Base(pa)

		if strings.HasSuffix(fname, ".mp") {
			fname += "4"
		}

		contentTyp = segment

	default:
		dir = pa

		if !strings.HasSuffix(dir, "/") {
			ctx.Header("Location", trailingSlashLocation(ctx.Request.URL.Path, ctx.Request.URL.RawQuery))
			ctx.Writer.WriteHeader(http.StatusFound)
			return
		}

		dir = dir[:len(dir)-1]
		contentTyp = index
	}

	switch contentTyp {
	case index:
		_, err := s.pathManager.FindPathConf(defs.PathFindPathConfReq{
			AccessRequest: defs.PathAccessRequest{
				Name:        dir,
				Query:       ctx.Request.URL.RawQuery,
				Publish:     false,
				Proto:       auth.ProtocolHLS,
				Credentials: httpp.Credentials(ctx.Request),
				IP:          net.ParseIP(ctx.ClientIP()),
			},
		})
		if err != nil {
			var terr *auth.Error
			if errors.As(err, &terr) {
				if terr.AskCredentials {
					ctx.Header("WWW-Authenticate", `Basic realm="mediamtx"`)
					s.writeErrorNoLog(ctx, http.StatusUnauthorized, fmt.Errorf("authentication error"))
					return
				}

				s.Log(logger.Info, "connection %v failed to authenticate: %v", httpp.RemoteAddr(ctx), terr.Wrapped)

				// wait some seconds to delay brute force attacks
				<-time.After(auth.PauseAfterError)

				s.writeErrorNoLog(ctx, http.StatusUnauthorized, fmt.Errorf("authentication error"))
				return
			}

			s.writeErrorNoLog(ctx, http.StatusInternalServerError, err)
			return
		}

		ctx.Header("Cache-Control", "max-age=3600")
		ctx.Header("Content-Type", "text/html")
		ctx.Writer.WriteHeader(http.StatusOK)
		ctx.Writer.Write(hlsIndex)

	case multivariantPlaylist:
		if ctx.Request.URL.Query().Get("cookieCheck") != "1" {
			http.SetCookie(ctx.Writer, &http.Cookie{
				Name:  "cookieCheck",
				Value: "1",
			})

			http.SetCookie(ctx.Writer, &http.Cookie{
				Name:        "cookieCheck",
				Value:       "1",
				SameSite:    http.SameSiteNoneMode,
				Secure:      true,
				Partitioned: true,
				HttpOnly:    true,
			})

			q := ctx.Request.URL.Query()
			q.Set("cookieCheck", "1")
			ctx.Request.URL.RawQuery = q.Encode()
			ctx.Writer.Header().Set("Location", sanitizeLocation(ctx.Request.URL.Path, ctx.Request.URL.RawQuery))

			ctx.Writer.WriteHeader(http.StatusFound)
			return
		}

		if _, err := ctx.Request.Cookie("cookieCheck"); err != nil && isIOS(ctx.Request.UserAgent()) {
			s.writeErrorNoLog(ctx, http.StatusBadRequest, fmt.Errorf("HLS on iOS requires the server to set and read cookies"))
			return
		}

		q := ctx.Request.URL.Query()
		q.Del("cookieCheck")
		ctx.Request.URL.RawQuery = q.Encode()

		sx := &session{
			remoteAddr:      httpp.RemoteAddr(ctx),
			pathName:        dir,
			externalCmdPool: s.parent.ExternalCmdPool,
			pathManager:     s.pathManager,
			server:          s.parent,
		}
		err := sx.initialize(ctx)
		if err != nil {
			var terr *auth.Error
			if errors.As(err, &terr) {
				if terr.AskCredentials {
					ctx.Header("WWW-Authenticate", `Basic realm="mediamtx"`)
					s.writeErrorNoLog(ctx, http.StatusUnauthorized, fmt.Errorf("authentication error"))
					return
				}

				s.Log(logger.Info, "connection %v failed to authenticate: %v", httpp.RemoteAddr(ctx), terr.Wrapped)

				// wait some seconds to delay brute force attacks
				<-time.After(auth.PauseAfterError)

				s.writeErrorNoLog(ctx, http.StatusUnauthorized, fmt.Errorf("authentication error"))
				return
			}

			var terr2 *defs.PathNoStreamAvailableError
			if errors.As(err, &terr2) {
				s.writeErrorNoLog(ctx, http.StatusNotFound, err)
				return
			}

			s.writeErrorNoLog(ctx, http.StatusInternalServerError, err)
			return
		}

		if cookie, err2 := ctx.Request.Cookie("cookieCheck"); err2 == nil && cookie.Value == "1" {
			http.SetCookie(ctx.Writer, &http.Cookie{
				Name:  sessionCookieName,
				Value: sx.secret.String(),
			})

			http.SetCookie(ctx.Writer, &http.Cookie{
				Name:        sessionCookieName,
				Value:       sx.secret.String(),
				SameSite:    http.SameSiteNoneMode,
				Secure:      true,
				Partitioned: true,
				HttpOnly:    true,
			})
		} else {
			q = ctx.Request.URL.Query()
			q.Set(sessionQueryParamName, sx.secret.String())
			ctx.Request.URL.RawQuery = q.Encode()
		}

		ctx.Writer = &responseWriterCounter{
			ResponseWriter: ctx.Writer,
			bytesSent:      &sx.bytesSent,
		}

		ctx.Request.URL.Path = fname

		err = sx.muxer.handleRequest(ctx)
		if err != nil {
			s.writeErrorNoLog(ctx, http.StatusInternalServerError, err)
			return
		}

	default:
		muxer, err := s.parent.getMuxer(serverGetMuxerReq{
			path:   dir,
			create: false,
		})
		if err != nil {
			// wait some seconds to delay brute force attacks
			<-time.After(auth.PauseAfterError)

			s.writeErrorNoLog(ctx, http.StatusUnauthorized, fmt.Errorf("authentication error"))
			return
		}

		sx := muxer.findSession(ctx)
		if sx == nil {
			// wait some seconds to delay brute force attacks
			<-time.After(auth.PauseAfterError)

			s.writeErrorNoLog(ctx, http.StatusUnauthorized, fmt.Errorf("authentication error"))
			return
		}

		ctx.Writer = &responseWriterCounter{
			ResponseWriter: ctx.Writer,
			bytesSent:      &sx.bytesSent,
		}

		ctx.Request.URL.Path = fname

		err = muxer.handleRequest(ctx)
		if err != nil {
			s.writeErrorNoLog(ctx, http.StatusInternalServerError, err)
			return
		}
	}
}
