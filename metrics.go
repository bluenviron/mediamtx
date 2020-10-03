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

	countClients := atomic.LoadInt64(m.p.countClients)
	countPublishers := atomic.LoadInt64(m.p.countPublishers)
	countReaders := atomic.LoadInt64(m.p.countReaders)
	countSourcesRtsp := atomic.LoadInt64(m.p.countSourcesRtsp)
	countSourcesRtspRunning := atomic.LoadInt64(m.p.countSourcesRtspRunning)
	countSourcesRtmp := atomic.LoadInt64(m.p.countSourcesRtmp)
	countSourcesRtmpRunning := atomic.LoadInt64(m.p.countSourcesRtmpRunning)

	out := ""
	out += fmt.Sprintf("rtsp_clients{state=\"idle\"} %d %v\n",
		countClients-countPublishers-countReaders, now)
	out += fmt.Sprintf("rtsp_clients{state=\"publishing\"} %d %v\n",
		countPublishers, now)
	out += fmt.Sprintf("rtsp_clients{state=\"reading\"} %d %v\n",
		countReaders, now)
	out += fmt.Sprintf("rtsp_sources{type=\"rtsp\",state=\"idle\"} %d %v\n",
		countSourcesRtsp-countSourcesRtspRunning, now)
	out += fmt.Sprintf("rtsp_sources{type=\"rtsp\",state=\"running\"} %d %v\n",
		countSourcesRtspRunning, now)
	out += fmt.Sprintf("rtsp_sources{type=\"rtmp\",state=\"idle\"} %d %v\n",
		countSourcesRtmp-countSourcesRtmpRunning, now)
	out += fmt.Sprintf("rtsp_sources{type=\"rtmp\",state=\"running\"} %d %v\n",
		countSourcesRtmpRunning, now)

	w.WriteHeader(http.StatusOK)
	io.WriteString(w, out)
}
