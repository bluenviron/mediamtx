package core

import (
	"net/http"

	// start pprof
	_ "net/http/pprof"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
)

type pprofParent interface {
	logger.Writer
}

type pprof struct {
	parent pprofParent

	httpServer *httpServer
}

func newPPROF(
	address string,
	readTimeout conf.StringDuration,
	parent pprofParent,
) (*pprof, error) {
	pp := &pprof{
		parent: parent,
	}

	var err error
	pp.httpServer, err = newHTTPServer(
		address,
		readTimeout,
		"",
		"",
		http.DefaultServeMux,
	)
	if err != nil {
		return nil, err
	}

	pp.Log(logger.Info, "listener opened on "+address)

	return pp, nil
}

func (pp *pprof) close() {
	pp.Log(logger.Info, "listener is closing")
	pp.httpServer.close()
}

func (pp *pprof) Log(level logger.Level, format string, args ...interface{}) {
	pp.parent.Log(level, "[pprof] "+format, args...)
}
