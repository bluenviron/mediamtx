package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

const (
	metricsAddress = ":9998"
)

type metrics struct {
	p        *program
	listener net.Listener
	mux      *http.ServeMux
	server   *http.Server
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

	m.p.log("[metrics] opened on " + metricsAddress)
	return m, nil
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
	now := time.Now().UnixNano() / 1000000

	countClients := atomic.LoadInt64(&m.p.countClients)
	countPublishers := atomic.LoadInt64(&m.p.countPublishers)
	countReaders := atomic.LoadInt64(&m.p.countReaders)
	countProxies := atomic.LoadInt64(&m.p.countProxies)
	countProxiesRunning := atomic.LoadInt64(&m.p.countProxiesRunning)

	out := ""
	out += fmt.Sprintf("rtsp_clients{state=\"idle\"} %d %v\n",
		countClients-countPublishers-countReaders, now)
	out += fmt.Sprintf("rtsp_clients{state=\"publishing\"} %d %v\n",
		countPublishers, now)
	out += fmt.Sprintf("rtsp_clients{state=\"reading\"} %d %v\n",
		countReaders, now)
	out += fmt.Sprintf("rtsp_proxies{state=\"idle\"} %d %v\n",
		countProxies-countProxiesRunning, now)
	out += fmt.Sprintf("rtsp_proxies{state=\"running\"} %d %v\n",
		countProxiesRunning, now)

	w.WriteHeader(http.StatusOK)
	io.WriteString(w, out)
}
