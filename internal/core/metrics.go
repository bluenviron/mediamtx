package core

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

func formatMetric(key string, value int64, nowUnix int64) string {
	return key + " " + strconv.FormatInt(value, 10) + " " +
		strconv.FormatInt(nowUnix, 10) + "\n"
}

type metricsParent interface {
	Log(logger.Level, string, ...interface{})
}

type metrics struct {
	stats *stats

	listener net.Listener
	mux      *http.ServeMux
	server   *http.Server
}

func newMetrics(
	address string,
	stats *stats,
	parent metricsParent,
) (*metrics, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	m := &metrics{
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

func (m *metrics) close() {
	m.server.Shutdown(context.Background())
}

func (m *metrics) run() {
	err := m.server.Serve(m.listener)
	if err != http.ErrServerClosed {
		panic(err)
	}
}

func (m *metrics) onMetrics(w http.ResponseWriter, req *http.Request) {
	nowUnix := time.Now().UnixNano() / 1000000

	countPublishers := atomic.LoadInt64(m.stats.CountPublishers)
	countReaders := atomic.LoadInt64(m.stats.CountReaders)
	countSourcesRTSP := atomic.LoadInt64(m.stats.CountSourcesRTSP)
	countSourcesRTSPRunning := atomic.LoadInt64(m.stats.CountSourcesRTSPRunning)
	countSourcesRTMP := atomic.LoadInt64(m.stats.CountSourcesRTMP)
	countSourcesRTMPRunning := atomic.LoadInt64(m.stats.CountSourcesRTMPRunning)

	out := ""

	out += formatMetric("rtsp_clients{state=\"publishing\"}",
		countPublishers, nowUnix)
	out += formatMetric("rtsp_clients{state=\"reading\"}",
		countReaders, nowUnix)

	out += formatMetric("rtsp_sources{type=\"rtsp\",state=\"idle\"}",
		countSourcesRTSP-countSourcesRTSPRunning, nowUnix)
	out += formatMetric("rtsp_sources{type=\"rtsp\",state=\"running\"}",
		countSourcesRTSPRunning, nowUnix)

	out += formatMetric("rtsp_sources{type=\"rtmp\",state=\"idle\"}",
		countSourcesRTMP-countSourcesRTMPRunning, nowUnix)
	out += formatMetric("rtsp_sources{type=\"rtmp\",state=\"running\"}",
		countSourcesRTMPRunning, nowUnix)

	w.WriteHeader(http.StatusOK)
	io.WriteString(w, out)
}
