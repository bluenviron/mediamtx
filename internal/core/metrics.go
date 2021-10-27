package core

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

func formatMetric(key string, value int64, nowUnix int64) string {
	return key + " " + strconv.FormatInt(value, 10) + " " +
		strconv.FormatInt(nowUnix, 10) + "\n"
}

type metricsPathManager interface {
	onAPIPathsList(req apiPathsListReq1) apiPathsListRes1
}

type metricsRTSPServer interface {
	onAPIRTSPSessionsList(req apiRTSPSessionsListReq) apiRTSPSessionsListRes
}

type metricsRTMPServer interface {
	onAPIRTMPConnsList(req apiRTMPConnsListReq) apiRTMPConnsListRes
}

type metricsParent interface {
	Log(logger.Level, string, ...interface{})
}

type metrics struct {
	listener net.Listener
	server   *http.Server

	mutex       sync.Mutex
	pathManager metricsPathManager
	rtspServer  metricsRTSPServer
	rtspsServer metricsRTSPServer
	rtmpServer  metricsRTMPServer
}

func newMetrics(
	address string,
	parent metricsParent,
) (*metrics, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	m := &metrics{
		listener: listener,
	}

	router := gin.New()
	router.GET("/metrics", m.onMetrics)

	m.server = &http.Server{Handler: router}

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

func (m *metrics) onMetrics(ctx *gin.Context) {
	nowUnix := time.Now().UnixNano() / 1000000

	out := ""

	res := m.pathManager.onAPIPathsList(apiPathsListReq1{})
	if res.Err == nil {
		readyCount := int64(0)
		notReadyCount := int64(0)

		for _, p := range res.Data.Items {
			if p.SourceReady {
				readyCount++
			} else {
				notReadyCount++
			}
		}

		out += formatMetric("paths{state=\"ready\"}",
			readyCount, nowUnix)
		out += formatMetric("paths{state=\"notReady\"}",
			notReadyCount, nowUnix)
	}

	if !interfaceIsEmpty(m.rtspServer) {
		res := m.rtspServer.onAPIRTSPSessionsList(apiRTSPSessionsListReq{})
		if res.Err == nil {
			idleCount := int64(0)
			readCount := int64(0)
			publishCount := int64(0)

			for _, i := range res.Data.Items {
				switch i.State {
				case "idle":
					idleCount++
				case "read":
					readCount++
				case "publish":
					publishCount++
				}
			}

			out += formatMetric("rtsp_sessions{state=\"idle\"}",
				idleCount, nowUnix)
			out += formatMetric("rtsp_sessions{state=\"read\"}",
				readCount, nowUnix)
			out += formatMetric("rtsp_sessions{state=\"publish\"}",
				publishCount, nowUnix)
		}
	}

	if !interfaceIsEmpty(m.rtspsServer) {
		res := m.rtspsServer.onAPIRTSPSessionsList(apiRTSPSessionsListReq{})
		if res.Err == nil {
			idleCount := int64(0)
			readCount := int64(0)
			publishCount := int64(0)

			for _, i := range res.Data.Items {
				switch i.State {
				case "idle":
					idleCount++
				case "read":
					readCount++
				case "publish":
					publishCount++
				}
			}

			out += formatMetric("rtsps_sessions{state=\"idle\"}",
				idleCount, nowUnix)
			out += formatMetric("rtsps_sessions{state=\"read\"}",
				readCount, nowUnix)
			out += formatMetric("rtsps_sessions{state=\"publish\"}",
				publishCount, nowUnix)
		}
	}

	if !interfaceIsEmpty(m.rtmpServer) {
		res := m.rtmpServer.onAPIRTMPConnsList(apiRTMPConnsListReq{})
		if res.Err == nil {
			idleCount := int64(0)
			readCount := int64(0)
			publishCount := int64(0)

			for _, i := range res.Data.Items {
				switch i.State {
				case "idle":
					idleCount++
				case "read":
					readCount++
				case "publish":
					publishCount++
				}
			}

			out += formatMetric("rtmp_conns{state=\"idle\"}",
				idleCount, nowUnix)
			out += formatMetric("rtmp_conns{state=\"read\"}",
				readCount, nowUnix)
			out += formatMetric("rtmp_conns{state=\"publish\"}",
				publishCount, nowUnix)
		}
	}

	ctx.Writer.WriteHeader(http.StatusOK)
	io.WriteString(ctx.Writer, out)
}

// onPathManagerSet is called by pathManager.
func (m *metrics) onPathManagerSet(s metricsPathManager) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.pathManager = s
}

// onRTSPServer is called by rtspServer (plain).
func (m *metrics) onRTSPServerSet(s metricsRTSPServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.rtspServer = s
}

// onRTSPServer is called by rtspServer (plain).
func (m *metrics) onRTSPSServerSet(s metricsRTSPServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.rtspsServer = s
}

// onRTMPServerSet is called by rtmpServer.
func (m *metrics) onRTMPServerSet(s metricsRTMPServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.rtmpServer = s
}
