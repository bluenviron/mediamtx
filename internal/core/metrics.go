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
	apiPathsList() pathAPIPathsListRes
}

type metricsRTSPServer interface {
	apiConnsList() rtspServerAPIConnsListRes
	apiSessionsList() rtspServerAPISessionsListRes
}

type metricsRTMPServer interface {
	apiConnsList() rtmpServerAPIConnsListRes
}

type metricsHLSServer interface {
	apiHLSMuxersList() hlsServerAPIMuxersListRes
}

type metricsParent interface {
	Log(logger.Level, string, ...interface{})
}

type metrics struct {
	parent metricsParent

	ln          net.Listener
	server      *http.Server
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
		parent: parent,
		ln:     ln,
	}

	router := gin.New()
	router.SetTrustedProxies(nil)
	router.GET("/metrics", m.onMetrics)

	m.server = &http.Server{Handler: router}

	m.log(logger.Info, "listener opened on "+address)

	go m.server.Serve(m.ln)

	return m, nil
}

func (m *metrics) close() {
	m.log(logger.Info, "listener is closing")
	m.server.Shutdown(context.Background())
	m.ln.Close() // in case Shutdown() is called before Serve()
}

func (m *metrics) log(level logger.Level, format string, args ...interface{}) {
	m.parent.Log(level, "[metrics] "+format, args...)
}

func (m *metrics) onMetrics(ctx *gin.Context) {
	out := ""

	res := m.pathManager.apiPathsList()
	if res.err == nil {
		for name, p := range res.data.Items {
			if p.SourceReady {
				out += metric("paths{name=\""+name+"\",state=\"ready\"}", 1)
			} else {
				out += metric("paths{name=\""+name+"\",state=\"notReady\"}", 1)
			}
		}
	}

	if !interfaceIsEmpty(m.rtspServer) {
		func() {
			res := m.rtspServer.apiConnsList()
			if res.err == nil {
				out += metric("rtsp_conns", int64(len(res.data.Items)))
			}
		}()

		func() {
			res := m.rtspServer.apiSessionsList()
			if res.err == nil {
				idleCount := int64(0)
				readCount := int64(0)
				publishCount := int64(0)

				for _, i := range res.data.Items {
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
		}()
	}

	if !interfaceIsEmpty(m.rtspsServer) {
		func() {
			res := m.rtspsServer.apiConnsList()
			if res.err == nil {
				out += metric("rtsps_conns", int64(len(res.data.Items)))
			}
		}()

		func() {
			res := m.rtspsServer.apiSessionsList()
			if res.err == nil {
				idleCount := int64(0)
				readCount := int64(0)
				publishCount := int64(0)

				for _, i := range res.data.Items {
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
		}()
	}

	if !interfaceIsEmpty(m.rtmpServer) {
		res := m.rtmpServer.apiConnsList()
		if res.err == nil {
			idleCount := int64(0)
			readCount := int64(0)
			publishCount := int64(0)

			for _, i := range res.data.Items {
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
		res := m.hlsServer.apiHLSMuxersList()
		if res.err == nil {
			for name := range res.data.Items {
				out += metric("hls_muxers{name=\""+name+"\"}", 1)
			}
		}
	}

	ctx.Writer.WriteHeader(http.StatusOK)
	io.WriteString(ctx.Writer, out)
}

// pathManagerSet is called by pathManager.
func (m *metrics) pathManagerSet(s metricsPathManager) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.pathManager = s
}

// rtspServerSet is called by rtspServer (plain).
func (m *metrics) rtspServerSet(s metricsRTSPServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.rtspServer = s
}

// rtspsServerSet is called by rtspServer (tls).
func (m *metrics) rtspsServerSet(s metricsRTSPServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.rtspsServer = s
}

// rtmpServerSet is called by rtmpServer.
func (m *metrics) rtmpServerSet(s metricsRTMPServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.rtmpServer = s
}

// hlsServerSet is called by hlsServer.
func (m *metrics) hlsServerSet(s metricsHLSServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.hlsServer = s
}
