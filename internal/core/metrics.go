package core

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

func metric(key string, value int64) string {
	return key + " " + strconv.FormatInt(value, 10) + "\n"
}

type metricsPathManager interface {
	onAPIPathsList(req apiPathsListReq) apiPathsListRes
}

type metricsRTSPServer interface {
	onAPIRTSPSessionsList(req apiRTSPSessionsListReq) apiRTSPSessionsListRes
}

type metricsRTMPServer interface {
	onAPIRTMPConnsList(req apiRTMPConnsListReq) apiRTMPConnsListRes
}

type metricsHLSServer interface {
	onAPIHLSMuxersList(req apiHLSMuxersListReq) apiHLSMuxersListRes
}

type metricsParent interface {
	Log(logger.Level, string, ...interface{})
}

type metrics struct {
	ln     net.Listener
	server *http.Server

	mutex       sync.Mutex
	pathManager metricsPathManager
	rtspServer  metricsRTSPServer
	rtspsServer metricsRTSPServer
	rtmpServer  metricsRTMPServer
	hlsServer   metricsHLSServer
}

func newMetrics(
	address string,
	parent metricsParent,
) (*metrics, error) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	m := &metrics{
		ln: ln,
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
	err := m.server.Serve(m.ln)
	if err != http.ErrServerClosed {
		panic(err)
	}
}

func (m *metrics) onMetrics(ctx *gin.Context) {
	out := ""

	res := m.pathManager.onAPIPathsList(apiPathsListReq{})
	if res.Err == nil {
		for name, p := range res.Data.Items {
			if p.SourceReady {
				out += metric("paths{name=\""+name+"\",state=\"ready\"}", 1)
			} else {
				out += metric("paths{name=\""+name+"\",state=\"notReady\"}", 1)
			}
		}
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

			out += metric("rtsp_sessions{state=\"idle\"}",
				idleCount)
			out += metric("rtsp_sessions{state=\"read\"}",
				readCount)
			out += metric("rtsp_sessions{state=\"publish\"}",
				publishCount)
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

			out += metric("rtsps_sessions{state=\"idle\"}",
				idleCount)
			out += metric("rtsps_sessions{state=\"read\"}",
				readCount)
			out += metric("rtsps_sessions{state=\"publish\"}",
				publishCount)
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

			out += metric("rtmp_conns{state=\"idle\"}",
				idleCount)
			out += metric("rtmp_conns{state=\"read\"}",
				readCount)
			out += metric("rtmp_conns{state=\"publish\"}",
				publishCount)
		}
	}

	if !interfaceIsEmpty(m.hlsServer) {
		res := m.hlsServer.onAPIHLSMuxersList(apiHLSMuxersListReq{})
		if res.Err == nil {
			for name := range res.Data.Items {
				out += metric("hls_muxers{name=\""+name+"\"}", 1)
			}
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

// onHLSServerSet is called by hlsServer.
func (m *metrics) onHLSServerSet(s metricsHLSServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.hlsServer = s
}
