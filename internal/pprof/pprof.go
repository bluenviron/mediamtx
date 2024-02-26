// Package pprof contains a pprof exporter.
package pprof

import (
	"net"
	"net/http"
	"strings"
	"time"

	// start pprof
	_ "net/http/pprof"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
)

type pprofParent interface {
	logger.Writer
}

// PPROF is a pprof exporter.
type PPROF struct {
	Address     string
	ReadTimeout conf.StringDuration
	AuthManager *auth.Manager
	Parent      pprofParent

	httpServer *httpp.WrappedServer
}

// Initialize initializes PPROF.
func (pp *PPROF) Initialize() error {
	network, address := restrictnetwork.Restrict("tcp", pp.Address)

	var err error
	pp.httpServer, err = httpp.NewWrappedServer(
		network,
		address,
		time.Duration(pp.ReadTimeout),
		"",
		"",
		pp,
		pp,
	)
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

func (pp *PPROF) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	user, pass, hasCredentials := r.BasicAuth()

	ip, _, _ := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))

	err := pp.AuthManager.Authenticate(&auth.Request{
		User:   user,
		Pass:   pass,
		IP:     net.ParseIP(ip),
		Action: conf.AuthActionMetrics,
	})
	if err != nil {
		if !hasCredentials {
			w.Header().Set("WWW-Authenticate", `Basic realm="mediamtx"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// wait some seconds to mitigate brute force attacks
		<-time.After(auth.PauseAfterError)

		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	http.DefaultServeMux.ServeHTTP(w, r)
}
