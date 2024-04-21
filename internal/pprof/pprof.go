// Package pprof contains a pprof exporter.
package pprof

import (
	"net"
	"net/http"
	"time"

	// start pprof
	_ "net/http/pprof"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
	"github.com/gin-gonic/gin"
)

type pprofAuthManager interface {
	Authenticate(req *auth.Request) error
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
	AllowOrigin    string
	TrustedProxies conf.IPNetworks
	ReadTimeout    conf.StringDuration
	AuthManager    pprofAuthManager
	Parent         pprofParent

	httpServer *httpp.WrappedServer
}

// Initialize initializes PPROF.
func (pp *PPROF) Initialize() error {
	router := gin.New()
	router.SetTrustedProxies(pp.TrustedProxies.ToTrustedProxies()) //nolint:errcheck
	router.NoRoute(pp.onRequest)

	network, address := restrictnetwork.Restrict("tcp", pp.Address)

	pp.httpServer = &httpp.WrappedServer{
		Network:     network,
		Address:     address,
		ReadTimeout: time.Duration(pp.ReadTimeout),
		Encryption:  pp.Encryption,
		ServerCert:  pp.ServerCert,
		ServerKey:   pp.ServerKey,
		Handler:     router,
		Parent:      pp,
	}
	err := pp.httpServer.Initialize()
	if err != nil {
		return err
	}

	pp.Log(logger.Info, "listener opened on "+address)

	return nil
}

// Close closes PPROF.
func (pp *PPROF) Close() {
	pp.Log(logger.Info, "listener is closing")
	pp.httpServer.Close()
}

// Log implements logger.Writer.
func (pp *PPROF) Log(level logger.Level, format string, args ...interface{}) {
	pp.Parent.Log(level, "[pprof] "+format, args...)
}

func (pp *PPROF) onRequest(ctx *gin.Context) {
	ctx.Writer.Header().Set("Access-Control-Allow-Origin", pp.AllowOrigin)
	ctx.Writer.Header().Set("Access-Control-Allow-Credentials", "true")

	user, pass, hasCredentials := ctx.Request.BasicAuth()

	err := pp.AuthManager.Authenticate(&auth.Request{
		User:   user,
		Pass:   pass,
		Query:  ctx.Request.URL.RawQuery,
		IP:     net.ParseIP(ctx.ClientIP()),
		Action: conf.AuthActionMetrics,
	})
	if err != nil {
		if !hasCredentials {
			ctx.Writer.Header().Set("WWW-Authenticate", `Basic realm="mediamtx"`)
			ctx.Writer.WriteHeader(http.StatusUnauthorized)
			return
		}

		// wait some seconds to mitigate brute force attacks
		<-time.After(auth.PauseAfterError)

		ctx.Writer.WriteHeader(http.StatusUnauthorized)
		return
	}

	http.DefaultServeMux.ServeHTTP(ctx.Writer, ctx.Request)
}
