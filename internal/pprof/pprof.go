// Package pprof contains a pprof exporter.
package pprof

import (
	"net/http"
	"time"

	// start pprof
	_ "net/http/pprof"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpserv"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
)

type pprofParent interface {
	logger.Writer
}

// PPROF is a pprof exporter.
type PPROF struct {
	Address     string
	ReadTimeout conf.StringDuration
	Parent      pprofParent

	httpServer *httpserv.WrappedServer
}

// Initialize initializes PPROF.
func (pp *PPROF) Initialize() error {
	network, address := restrictnetwork.Restrict("tcp", pp.Address)

	var err error
	pp.httpServer, err = httpserv.NewWrappedServer(
		network,
		address,
		time.Duration(pp.ReadTimeout),
		"",
		"",
		http.DefaultServeMux,
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
