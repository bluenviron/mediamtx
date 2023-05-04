package core

import (
	"context"
	"log"
	"net"
	"net/http"
	"time"

	// start pprof
	_ "net/http/pprof"

	"github.com/aler9/mediamtx/internal/conf"
	"github.com/aler9/mediamtx/internal/logger"
)

type pprofParent interface {
	logger.Writer
}

type pprof struct {
	parent pprofParent

	ln         net.Listener
	httpServer *http.Server
}

func newPPROF(
	address string,
	readTimeout conf.StringDuration,
	parent pprofParent,
) (*pprof, error) {
	ln, err := net.Listen(restrictNetwork("tcp", address))
	if err != nil {
		return nil, err
	}

	pp := &pprof{
		parent: parent,
		ln:     ln,
	}

	pp.httpServer = &http.Server{
		Handler:           http.DefaultServeMux,
		ReadHeaderTimeout: time.Duration(readTimeout),
		ErrorLog:          log.New(&nilWriter{}, "", 0),
	}

	pp.Log(logger.Info, "listener opened on "+address)

	go pp.httpServer.Serve(pp.ln)

	return pp, nil
}

func (pp *pprof) close() {
	pp.Log(logger.Info, "listener is closing")
	pp.httpServer.Shutdown(context.Background())
	pp.ln.Close() // in case Shutdown() is called before Serve()
}

func (pp *pprof) Log(level logger.Level, format string, args ...interface{}) {
	pp.parent.Log(level, "[pprof] "+format, args...)
}
