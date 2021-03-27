package pprof

import (
	"context"
	"net"
	"net/http"
	"strconv"

	// start pprof
	_ "net/http/pprof"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

// Parent is implemented by program.
type Parent interface {
	Log(logger.Level, string, ...interface{})
}

// PPROF is a performance metrics exporter.
type PPROF struct {
	listener net.Listener
	server   *http.Server
}

// New allocates a PPROF.
func New(
	listenIP string,
	port int,
	parent Parent,
) (*PPROF, error) {
	address := listenIP + ":" + strconv.FormatInt(int64(port), 10)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	pp := &PPROF{
		listener: listener,
	}

	pp.server = &http.Server{
		Handler: http.DefaultServeMux,
	}

	parent.Log(logger.Info, "[pprof] opened on "+address)

	go pp.run()
	return pp, nil
}

// Close closes a PPROF.
func (pp *PPROF) Close() {
	pp.server.Shutdown(context.Background())
}

func (pp *PPROF) run() {
	err := pp.server.Serve(pp.listener)
	if err != http.ErrServerClosed {
		panic(err)
	}
}
