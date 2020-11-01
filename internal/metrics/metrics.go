package metrics

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/aler9/rtsp-simple-server/internal/stats"
)

const (
	address = ":9998"
)

type Parent interface {
	Log(string, ...interface{})
}

type Metrics struct {
	stats *stats.Stats

	listener net.Listener
	mux      *http.ServeMux
	server   *http.Server
}

func New(stats *stats.Stats, parent Parent) (*Metrics, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	m := &Metrics{
		stats:    stats,
		listener: listener,
	}

	m.mux = http.NewServeMux()
	m.mux.HandleFunc("/metrics", m.onMetrics)

	m.server = &http.Server{
		Handler: m.mux,
	}

	parent.Log("[metrics] opened on " + address)

	go m.run()
	return m, nil
}

func (m *Metrics) Close() {
	m.server.Shutdown(context.Background())
}

func (m *Metrics) run() {
	err := m.server.Serve(m.listener)
	if err != http.ErrServerClosed {
		panic(err)
	}
}

func (m *Metrics) onMetrics(w http.ResponseWriter, req *http.Request) {
	now := time.Now().UnixNano() / 1000000

	countClients := atomic.LoadInt64(m.stats.CountClients)
	countPublishers := atomic.LoadInt64(m.stats.CountPublishers)
	countReaders := atomic.LoadInt64(m.stats.CountReaders)
	countSourcesRtsp := atomic.LoadInt64(m.stats.CountSourcesRtsp)
	countSourcesRtspRunning := atomic.LoadInt64(m.stats.CountSourcesRtspRunning)
	countSourcesRtmp := atomic.LoadInt64(m.stats.CountSourcesRtmp)
	countSourcesRtmpRunning := atomic.LoadInt64(m.stats.CountSourcesRtmpRunning)

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
