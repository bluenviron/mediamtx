package main

import (
	"net"
	"net/http"
	_ "net/http/pprof"
)

const (
	pprofAddress = ":9998"
)

type pprof struct {
	listener net.Listener
	server   *http.Server
}

func newPprof(p *program) (*pprof, error) {
	listener, err := net.Listen("tcp", pprofAddress)
	if err != nil {
		return nil, err
	}

	pp := &pprof{
		listener: listener,
	}

	pp.server = &http.Server{
		Handler: http.DefaultServeMux,
	}

	p.log("[pprof] opened on " + pprofAddress)
	return pp, nil
}

func (pp *pprof) run() {
	err := pp.server.Serve(pp.listener)
	if err != http.ErrServerClosed {
		panic(err)
	}
}
