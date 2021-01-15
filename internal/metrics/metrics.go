package metrics

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

const (
	port = 9998
)

func formatMetric(key string, value int64, nowUnix int64) string {
	return key + " " + strconv.FormatInt(value, 10) + " " +
		strconv.FormatInt(nowUnix, 10) + "\n"
}

// Parent is implemented by program.
type Parent interface {
	Log(logger.Level, string, ...interface{})
}

// Metrics is a metrics exporter.
type Metrics struct {
	stats *stats.Stats

	listener net.Listener
	mux      *http.ServeMux
	server   *http.Server
}

// New allocates a metrics.
func New(
	listenIP string,
	stats *stats.Stats,
	parent Parent,
) (*Metrics, error) {

	address := listenIP + ":" + strconv.FormatInt(port, 10)
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

	parent.Log(logger.Info, "[metrics] opened on "+address)

	go m.run()
	return m, nil
}

// Close closes a Metrics.
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
	nowUnix := time.Now().UnixNano() / 1000000

	countClients := atomic.LoadInt64(m.stats.CountClients)
	countPublishers := atomic.LoadInt64(m.stats.CountPublishers)
	countReaders := atomic.LoadInt64(m.stats.CountReaders)
	countSourcesRtsp := atomic.LoadInt64(m.stats.CountSourcesRtsp)
	countSourcesRtspRunning := atomic.LoadInt64(m.stats.CountSourcesRtspRunning)
	countSourcesRtmp := atomic.LoadInt64(m.stats.CountSourcesRtmp)
	countSourcesRtmpRunning := atomic.LoadInt64(m.stats.CountSourcesRtmpRunning)

	out := ""
	out += formatMetric("rtsp_clients{state=\"idle\"}",
		countClients-countPublishers-countReaders, nowUnix)
	out += formatMetric("rtsp_clients{state=\"publishing\"}",
		countPublishers, nowUnix)
	out += formatMetric("rtsp_clients{state=\"reading\"}",
		countReaders, nowUnix)
	out += formatMetric("rtsp_sources{type=\"rtsp\",state=\"idle\"}",
		countSourcesRtsp-countSourcesRtspRunning, nowUnix)
	out += formatMetric("rtsp_sources{type=\"rtsp\",state=\"running\"}",
		countSourcesRtspRunning, nowUnix)
	out += formatMetric("rtsp_sources{type=\"rtmp\",state=\"idle\"}",
		countSourcesRtmp-countSourcesRtmpRunning, nowUnix)
	out += formatMetric("rtsp_sources{type=\"rtmp\",state=\"running\"}",
		countSourcesRtmpRunning, nowUnix)

	w.WriteHeader(http.StatusOK)
	io.WriteString(w, out)
}
