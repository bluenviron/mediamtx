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

// Pprof is a performance metrics exporter.
type Pprof struct {
	listener net.Listener
	server   *http.Server
}

// New allocates a Pprof.
func New(
	listenIP string,
	port int,
	parent Parent,
) (*Pprof, error) {
	address := listenIP + ":" + strconv.FormatInt(int64(port), 10)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	pp := &Pprof{
		listener: listener,
	}

	pp.server = &http.Server{
		Handler: http.DefaultServeMux,
	}

	parent.Log(logger.Info, "[pprof] opened on "+address)

	go pp.run()
	return pp, nil
}

// Close closes a Pprof.
func (pp *Pprof) Close() {
	pp.server.Shutdown(context.Background())
}

func (pp *Pprof) run() {
	err := pp.server.Serve(pp.listener)
	if err != http.ErrServerClosed {
		panic(err)
	}
}
