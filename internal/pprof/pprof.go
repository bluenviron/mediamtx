// Package pprof contains a pprof exporter.
package pprof //nolint:revive

import (
	"net"
	"net/http"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
)

type pprofAuthManager interface {
	Authenticate(req *auth.Request) *auth.Error
}

type pprofParent interface {
	logger.Writer
}

// PPROF is a pprof exporter.
type PPROF struct {
	Address        string
	Encryption     bool
	ServerKey      string
	ServerCert     string
	AllowOrigins   []string
	TrustedProxies conf.IPNetworks
	ReadTimeout    conf.Duration
	WriteTimeout   conf.Duration
	AuthManager    pprofAuthManager
	Parent         pprofParent

	httpServer *httpp.Server
}

// Initialize initializes PPROF.
func (pp *PPROF) Initialize() error {
	router := gin.New()
	router.SetTrustedProxies(pp.TrustedProxies.ToTrustedProxies()) //nolint:errcheck

	router.Use(pp.middlewarePreflightRequests)
	router.Use(pp.middlewareAuth)

	pprof.Register(router)

	pp.httpServer = &httpp.Server{
		Address:      pp.Address,
		AllowOrigins: pp.AllowOrigins,
		ReadTimeout:  time.Duration(pp.ReadTimeout),
		WriteTimeout: time.Duration(pp.WriteTimeout),
		Encryption:   pp.Encryption,
		ServerCert:   pp.ServerCert,
		ServerKey:    pp.ServerKey,
		Handler:      router,
		Parent:       pp,
	}
	err := pp.httpServer.Initialize()
	if err != nil {
		return err
	}

	pp.Log(logger.Info, "listener opened on "+pp.Address)

	return nil
}

// Close closes PPROF.
func (pp *PPROF) Close() {
	pp.Log(logger.Info, "listener is closing")
	pp.httpServer.Close()
}

// Log implements logger.Writer.
func (pp *PPROF) Log(level logger.Level, format string, args ...any) {
	pp.Parent.Log(level, "[pprof] "+format, args...)
}

func (pp *PPROF) middlewarePreflightRequests(ctx *gin.Context) {
	if ctx.Request.Method == http.MethodOptions &&
		ctx.Request.Header.Get("Access-Control-Request-Method") != "" {
		ctx.Header("Access-Control-Allow-Methods", "OPTIONS, GET")
		ctx.Header("Access-Control-Allow-Headers", "Authorization")
		ctx.AbortWithStatus(http.StatusNoContent)
		return
	}
}

func (pp *PPROF) middlewareAuth(ctx *gin.Context) {
	req := &auth.Request{
		Action:      conf.AuthActionPprof,
		Query:       ctx.Request.URL.RawQuery,
		Credentials: httpp.Credentials(ctx.Request),
		IP:          net.ParseIP(ctx.ClientIP()),
	}

	err := pp.AuthManager.Authenticate(req)
	if err != nil {
		if err.AskCredentials {
			ctx.Header("WWW-Authenticate", `Basic realm="mediamtx"`)
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, &defs.APIError{
				Status: "error",
				Error:  "authentication error",
			})
			return
		}

		pp.Log(logger.Info, "connection %v failed to authenticate: %v", httpp.RemoteAddr(ctx), err.Wrapped)

		// wait some seconds to delay brute force attacks
		<-time.After(auth.PauseAfterError)

		ctx.AbortWithStatusJSON(http.StatusUnauthorized, &defs.APIError{
			Status: "error",
			Error:  "authentication error",
		})
		return
	}
}
