package core

import (
	"context"
	"net"
	"net/http"

	// start pprof
	_ "net/http/pprof"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type pprofParent interface {
	Log(logger.Level, string, ...interface{})
}

type pprof struct {
	ln     net.Listener
	server *http.Server
}

func newPPROF(
	address string,
	parent pprofParent,
) (*pprof, error) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	pp := &pprof{
		ln: ln,
	}

	pp.server = &http.Server{
		Handler: http.DefaultServeMux,
	}

	parent.Log(logger.Info, "[pprof] opened on "+address)

	go pp.run()

	return pp, nil
}

func (pp *pprof) close() {
	pp.server.Shutdown(context.Background())
}

func (pp *pprof) run() {
	err := pp.server.Serve(pp.ln)
	if err != http.ErrServerClosed {
		panic(err)
	}
}
