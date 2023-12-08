package core

import (
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpserv"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
)

func metric(key string, tags string, value int64) string {
	return key + tags + " " + strconv.FormatInt(value, 10) + "\n"
}

type metricsParent interface {
	logger.Writer
}

type metrics struct {
	Address     string
	ReadTimeout conf.StringDuration
	Parent      metricsParent

	httpServer   *httpserv.WrappedServer
	mutex        sync.Mutex
	pathManager  apiPathManager
	rtspServer   apiRTSPServer
	rtspsServer  apiRTSPServer
	rtmpServer   apiRTMPServer
	rtmpsServer  apiRTMPServer
	srtServer    apiSRTServer
	hlsManager   apiHLSServer
	webRTCServer apiWebRTCServer
}

func (m *metrics) initialize() error {
	router := gin.New()
	router.SetTrustedProxies(nil) //nolint:errcheck

	router.GET("/metrics", m.onMetrics)

	network, address := restrictnetwork.Restrict("tcp", m.Address)

	var err error
	m.httpServer, err = httpserv.NewWrappedServer(
		network,
		address,
		time.Duration(m.ReadTimeout),
		"",
		"",
		router,
		m,
	)
	if err != nil {
		return err
	}

	m.Log(logger.Info, "listener opened on "+address)

	return nil
}

func (m *metrics) close() {
	m.Log(logger.Info, "listener is closing")
	m.httpServer.Close()
}

// Log implements logger.Writer.
func (m *metrics) Log(level logger.Level, format string, args ...interface{}) {
	m.Parent.Log(level, "[metrics] "+format, args...)
}

func (m *metrics) onMetrics(ctx *gin.Context) {
	out := ""

	data, err := m.pathManager.apiPathsList()
	if err == nil && len(data.Items) != 0 {
		for _, i := range data.Items {
			var state string
			if i.Ready {
				state = "ready"
			} else {
				state = "notReady"
			}

			tags := "{name=\"" + i.Name + "\",state=\"" + state + "\"}"
			out += metric("paths", tags, 1)
			out += metric("paths_bytes_received", tags, int64(i.BytesReceived))
			out += metric("paths_bytes_sent", tags, int64(i.BytesSent))
		}
	} else {
		out += metric("paths", "", 0)
	}

	if !interfaceIsEmpty(m.hlsManager) {
		data, err := m.hlsManager.APIMuxersList()
		if err == nil && len(data.Items) != 0 {
			for _, i := range data.Items {
				tags := "{name=\"" + i.Path + "\"}"
				out += metric("hls_muxers", tags, 1)
				out += metric("hls_muxers_bytes_sent", tags, int64(i.BytesSent))
			}
		} else {
			out += metric("hls_muxers", "", 0)
			out += metric("hls_muxers_bytes_sent", "", 0)
		}
	}

	if !interfaceIsEmpty(m.rtspServer) { //nolint:dupl
		func() {
			data, err := m.rtspServer.APIConnsList()
			if err == nil && len(data.Items) != 0 {
				for _, i := range data.Items {
					tags := "{id=\"" + i.ID.String() + "\"}"
					out += metric("rtsp_conns", tags, 1)
					out += metric("rtsp_conns_bytes_received", tags, int64(i.BytesReceived))
					out += metric("rtsp_conns_bytes_sent", tags, int64(i.BytesSent))
				}
			} else {
				out += metric("rtsp_conns", "", 0)
				out += metric("rtsp_conns_bytes_received", "", 0)
				out += metric("rtsp_conns_bytes_sent", "", 0)
			}
		}()

		func() {
			data, err := m.rtspServer.APISessionsList()
			if err == nil && len(data.Items) != 0 {
				for _, i := range data.Items {
					tags := "{id=\"" + i.ID.String() + "\",state=\"" + string(i.State) + "\"}"
					out += metric("rtsp_sessions", tags, 1)
					out += metric("rtsp_sessions_bytes_received", tags, int64(i.BytesReceived))
					out += metric("rtsp_sessions_bytes_sent", tags, int64(i.BytesSent))
				}
			} else {
				out += metric("rtsp_sessions", "", 0)
				out += metric("rtsp_sessions_bytes_received", "", 0)
				out += metric("rtsp_sessions_bytes_sent", "", 0)
			}
		}()
	}

	if !interfaceIsEmpty(m.rtspsServer) { //nolint:dupl
		func() {
			data, err := m.rtspsServer.APIConnsList()
			if err == nil && len(data.Items) != 0 {
				for _, i := range data.Items {
					tags := "{id=\"" + i.ID.String() + "\"}"
					out += metric("rtsps_conns", tags, 1)
					out += metric("rtsps_conns_bytes_received", tags, int64(i.BytesReceived))
					out += metric("rtsps_conns_bytes_sent", tags, int64(i.BytesSent))
				}
			} else {
				out += metric("rtsps_conns", "", 0)
				out += metric("rtsps_conns_bytes_received", "", 0)
				out += metric("rtsps_conns_bytes_sent", "", 0)
			}
		}()

		func() {
			data, err := m.rtspsServer.APISessionsList()
			if err == nil && len(data.Items) != 0 {
				for _, i := range data.Items {
					tags := "{id=\"" + i.ID.String() + "\",state=\"" + string(i.State) + "\"}"
					out += metric("rtsps_sessions", tags, 1)
					out += metric("rtsps_sessions_bytes_received", tags, int64(i.BytesReceived))
					out += metric("rtsps_sessions_bytes_sent", tags, int64(i.BytesSent))
				}
			} else {
				out += metric("rtsps_sessions", "", 0)
				out += metric("rtsps_sessions_bytes_received", "", 0)
				out += metric("rtsps_sessions_bytes_sent", "", 0)
			}
		}()
	}

	if !interfaceIsEmpty(m.rtmpServer) {
		data, err := m.rtmpServer.APIConnsList()
		if err == nil && len(data.Items) != 0 {
			for _, i := range data.Items {
				tags := "{id=\"" + i.ID.String() + "\",state=\"" + string(i.State) + "\"}"
				out += metric("rtmp_conns", tags, 1)
				out += metric("rtmp_conns_bytes_received", tags, int64(i.BytesReceived))
				out += metric("rtmp_conns_bytes_sent", tags, int64(i.BytesSent))
			}
		} else {
			out += metric("rtmp_conns", "", 0)
			out += metric("rtmp_conns_bytes_received", "", 0)
			out += metric("rtmp_conns_bytes_sent", "", 0)
		}
	}

	if !interfaceIsEmpty(m.rtmpsServer) {
		data, err := m.rtmpsServer.APIConnsList()
		if err == nil && len(data.Items) != 0 {
			for _, i := range data.Items {
				tags := "{id=\"" + i.ID.String() + "\",state=\"" + string(i.State) + "\"}"
				out += metric("rtmps_conns", tags, 1)
				out += metric("rtmps_conns_bytes_received", tags, int64(i.BytesReceived))
				out += metric("rtmps_conns_bytes_sent", tags, int64(i.BytesSent))
			}
		} else {
			out += metric("rtmps_conns", "", 0)
			out += metric("rtmps_conns_bytes_received", "", 0)
			out += metric("rtmps_conns_bytes_sent", "", 0)
		}
	}

	if !interfaceIsEmpty(m.srtServer) {
		data, err := m.srtServer.APIConnsList()
		if err == nil && len(data.Items) != 0 {
			for _, i := range data.Items {
				tags := "{id=\"" + i.ID.String() + "\",state=\"" + string(i.State) + "\"}"
				out += metric("srt_conns", tags, 1)
				out += metric("srt_conns_bytes_received", tags, int64(i.BytesReceived))
				out += metric("srt_conns_bytes_sent", tags, int64(i.BytesSent))
			}
		} else {
			out += metric("srt_conns", "", 0)
			out += metric("srt_conns_bytes_received", "", 0)
			out += metric("srt_conns_bytes_sent", "", 0)
		}
	}

	if !interfaceIsEmpty(m.webRTCServer) {
		data, err := m.webRTCServer.APISessionsList()
		if err == nil && len(data.Items) != 0 {
			for _, i := range data.Items {
				tags := "{id=\"" + i.ID.String() + "\",state=\"" + string(i.State) + "\"}"
				out += metric("webrtc_sessions", tags, 1)
				out += metric("webrtc_sessions_bytes_received", tags, int64(i.BytesReceived))
				out += metric("webrtc_sessions_bytes_sent", tags, int64(i.BytesSent))
			}
		} else {
			out += metric("webrtc_sessions", "", 0)
			out += metric("webrtc_sessions_bytes_received", "", 0)
			out += metric("webrtc_sessions_bytes_sent", "", 0)
		}
	}

	ctx.Writer.WriteHeader(http.StatusOK)
	io.WriteString(ctx.Writer, out) //nolint:errcheck
}

// setPathManager is called by core.
func (m *metrics) setPathManager(s apiPathManager) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.pathManager = s
}

// setHLSServer is called by core.
func (m *metrics) setHLSServer(s apiHLSServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.hlsManager = s
}

// setRTSPServer is called by core.
func (m *metrics) setRTSPServer(s apiRTSPServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.rtspServer = s
}

// setRTSPSServer is called by core.
func (m *metrics) setRTSPSServer(s apiRTSPServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.rtspsServer = s
}

// setRTMPServer is called by core.
func (m *metrics) setRTMPServer(s apiRTMPServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.rtmpServer = s
}

// setRTMPSServer is called by core.
func (m *metrics) setRTMPSServer(s apiRTMPServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.rtmpsServer = s
}

// setSRTServer is called by core.
func (m *metrics) setSRTServer(s apiSRTServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.srtServer = s
}

// setWebRTCServer is called by core.
func (m *metrics) setWebRTCServer(s apiWebRTCServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.webRTCServer = s
}
