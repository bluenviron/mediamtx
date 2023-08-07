package core

import (
	"net/http"
	"time"

	// start pprof
	_ "net/http/pprof"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/httpserv"
	"github.com/bluenviron/mediamtx/internal/logger"
)

type pprofParent interface {
	logger.Writer
}

type pprof struct {
	parent pprofParent

	httpServer *httpserv.WrappedServer
}

func newPPROF(
	address string,
	readTimeout conf.StringDuration,
	parent pprofParent,
) (*pprof, error) {
	pp := &pprof{
		parent: parent,
	}

	network, address := restrictNetwork("tcp", address)

	var err error
	pp.httpServer, err = httpserv.NewWrappedServer(
		network,
		address,
		time.Duration(readTimeout),
		"",
		"",
		http.DefaultServeMux,
		pp,
	)
	if err != nil {
		return nil, err
	}

	pp.Log(logger.Info, "listener opened on "+address)

	return pp, nil
}

func (pp *pprof) close() {
	pp.Log(logger.Info, "listener is closing")
	pp.httpServer.Close()
}

func (pp *pprof) Log(level logger.Level, format string, args ...interface{}) {
	pp.parent.Log(level, "[pprof] "+format, args...)
}
