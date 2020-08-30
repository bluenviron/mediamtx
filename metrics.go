package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const (
	metricsAddress = ":9998"
)

type metricsData struct {
	clientCount    int
	publisherCount int
	readerCount    int
}

type metrics struct {
	p        *program
	mux      *http.ServeMux
	server   *http.Server
	listener net.Listener
}

func newMetrics(p *program) (*metrics, error) {
	listener, err := net.Listen("tcp", metricsAddress)
	if err != nil {
		return nil, err
	}

	m := &metrics{
		p:        p,
		listener: listener,
	}

	m.mux = http.NewServeMux()
	m.mux.HandleFunc("/metrics", m.onMetrics)

	m.server = &http.Server{
		Handler: m.mux,
	}

	m.log("opened on " + metricsAddress)
	return m, nil
}

func (m *metrics) log(format string, args ...interface{}) {
	m.p.log("[metrics] "+format, args...)
}

func (m *metrics) run() {
	err := m.server.Serve(m.listener)
	if err != http.ErrServerClosed {
		panic(err)
	}
}

func (m *metrics) close() {
	m.server.Shutdown(context.Background())
}

func (m *metrics) onMetrics(w http.ResponseWriter, req *http.Request) {
	res := make(chan *metricsData)
	m.p.events <- programEventMetrics{res}
	data := <-res

	if data == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	out := ""
	now := time.Now().UnixNano() / 1000000

	out += fmt.Sprintf("clients %d %v\n", data.clientCount, now)
	out += fmt.Sprintf("publishers %d %v\n", data.publisherCount, now)
	out += fmt.Sprintf("readers %d %v\n", data.readerCount, now)

	w.WriteHeader(http.StatusOK)
	io.WriteString(w, out)
}
