// Package metrics contains the metrics provider.
package metrics //nolint:revive

import (
	"fmt"
	"net"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
)

func interfaceIsEmpty(i any) bool {
	return reflect.ValueOf(i).Kind() != reflect.Pointer || reflect.ValueOf(i).IsNil()
}

func sortedKeys(paths map[string]string) []string {
	ret := make([]string, len(paths))
	i := 0
	for name := range paths {
		ret[i] = name
		i++
	}
	sort.Strings(ret)
	return ret
}

func tags(m map[string]string) string {
	var b strings.Builder
	b.WriteByte('{')
	first := true
	for _, k := range sortedKeys(m) {
		if !first {
			b.WriteByte(',')
		}
		first = false
		b.WriteString(k)
		b.WriteString("=\"")
		b.WriteString(m[k])
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}

func metric(out *strings.Builder, key string, tags string, value int64) {
	out.WriteString(key)
	out.WriteString(tags)
	out.WriteByte(' ')
	out.WriteString(strconv.FormatInt(value, 10))
	out.WriteByte('\n')
}

func metricFloat(out *strings.Builder, key string, tags string, value float64) {
	out.WriteString(key)
	out.WriteString(tags)
	out.WriteByte(' ')
	out.WriteString(strconv.FormatFloat(value, 'f', -1, 64))
	out.WriteByte('\n')
}

type metricsAuthManager interface {
	Authenticate(req *auth.Request) (string, *auth.Error)
}

type metricsParent interface {
	logger.Writer
}

// Metrics is a metrics provider.
type Metrics struct {
	Address        string
	DumpPackets    bool
	Encryption     bool
	ServerKey      string
	ServerCert     string
	AllowOrigins   []string
	TrustedProxies conf.IPNetworks
	ReadTimeout    conf.Duration
	WriteTimeout   conf.Duration
	AuthManager    metricsAuthManager
	Parent         metricsParent

	httpServer   *httpp.Server
	mutex        sync.Mutex
	pathManager  defs.APIPathManager
	hlsServer    defs.APIHLSServer
	rtspServer   defs.APIRTSPServer
	rtspsServer  defs.APIRTSPServer
	rtmpServer   defs.APIRTMPServer
	rtmpsServer  defs.APIRTMPServer
	srtServer    defs.APISRTServer
	webRTCServer defs.APIWebRTCServer
}

// Initialize initializes metrics.
func (m *Metrics) Initialize() error {
	router := gin.New()
	router.SetTrustedProxies(m.TrustedProxies.ToTrustedProxies()) //nolint:errcheck

	router.Use(m.middlewarePreflightRequests)
	router.Use(m.middlewareAuth)

	router.GET("/metrics", m.onMetrics)

	m.httpServer = &httpp.Server{
		Address:           m.Address,
		AllowOrigins:      m.AllowOrigins,
		DumpPackets:       m.DumpPackets,
		DumpPacketsPrefix: "metrics_server_conn",
		ReadTimeout:       time.Duration(m.ReadTimeout),
		WriteTimeout:      time.Duration(m.WriteTimeout),
		Encryption:        m.Encryption,
		ServerCert:        m.ServerCert,
		ServerKey:         m.ServerKey,
		Handler:           router,
		Parent:            m,
	}
	err := m.httpServer.Initialize()
	if err != nil {
		return err
	}

	str := "listener opened on " + m.Address
	if !m.Encryption {
		str += " (TCP/HTTP)"
	} else {
		str += " (TCP/HTTPS)"
	}
	m.Log(logger.Info, str)

	return nil
}

// Close closes Metrics.
func (m *Metrics) Close() {
	m.Log(logger.Info, "listener is closing")
	m.httpServer.Close()
}

// Log implements logger.Writer.
func (m *Metrics) Log(level logger.Level, format string, args ...any) {
	m.Parent.Log(level, "[metrics] "+format, args...)
}

func (m *Metrics) middlewarePreflightRequests(ctx *gin.Context) {
	if ctx.Request.Method == http.MethodOptions &&
		ctx.Request.Header.Get("Access-Control-Request-Method") != "" {
		ctx.Header("Access-Control-Allow-Methods", "OPTIONS, GET")
		ctx.Header("Access-Control-Allow-Headers", "Authorization")
		ctx.AbortWithStatus(http.StatusNoContent)
		return
	}
}

func (m *Metrics) writeErrorNoLog(ctx *gin.Context, status int, err error) {
	ctx.AbortWithStatusJSON(status, &defs.APIError{
		Status: defs.APIErrorStatusError,
		Error:  err.Error(),
	})
}

func (m *Metrics) middlewareAuth(ctx *gin.Context) {
	req := &auth.Request{
		Action:      conf.AuthActionMetrics,
		Query:       ctx.Request.URL.RawQuery,
		Credentials: httpp.Credentials(ctx.Request),
		IP:          net.ParseIP(ctx.ClientIP()),
	}

	_, err := m.AuthManager.Authenticate(req)
	if err != nil {
		if err.AskCredentials {
			ctx.Header("WWW-Authenticate", `Basic realm="mediamtx"`)
			m.writeErrorNoLog(ctx, http.StatusUnauthorized, fmt.Errorf("authentication error"))
			return
		}

		m.Log(logger.Info, "connection %v failed to authenticate: %v", httpp.RemoteAddr(ctx), err.Wrapped)

		// wait some seconds to delay brute force attacks
		<-time.After(auth.PauseAfterError)

		m.writeErrorNoLog(ctx, http.StatusUnauthorized, fmt.Errorf("authentication error"))
		return
	}
}

func (m *Metrics) onMetrics(ctx *gin.Context) {
	typ := ctx.Query("type")
	pathFilter := ctx.Query("path")
	hlsMuxerFilter := ctx.Query("hls_muxer")
	rtspConnFilter := ctx.Query("rtsp_conn")
	rtspSessionFilter := ctx.Query("rtsp_session")
	rtspsConnFilter := ctx.Query("rtsps_conn")
	rtspsSessionFilter := ctx.Query("rtsps_session")
	rtmpConnFilter := ctx.Query("rtmp_conn")
	rtmpsConnFilter := ctx.Query("rtmps_conn")
	srtConnFilter := ctx.Query("srt_conn")
	webrtcSessionFilter := ctx.Query("webrtc_session")

	anyFilterActive := pathFilter != "" ||
		hlsMuxerFilter != "" ||
		rtspConnFilter != "" ||
		rtspSessionFilter != "" ||
		rtspsConnFilter != "" ||
		rtspsSessionFilter != "" ||
		rtmpConnFilter != "" ||
		rtmpsConnFilter != "" ||
		srtConnFilter != "" ||
		webrtcSessionFilter != ""

	var out strings.Builder

	if (typ == "" || typ == "paths") && (!anyFilterActive || pathFilter != "") {
		data, err := m.pathManager.APIPathsList()
		if err == nil && len(data.Items) != 0 {
			for _, i := range data.Items {
				if pathFilter == "" || pathFilter == i.Name {
					var state string
					if i.Ready {
						state = "ready"
					} else {
						state = "notReady"
					}

					ta := tags(map[string]string{
						"name":  i.Name,
						"state": state,
					})
					metric(&out, "paths", ta, 1)
					metric(&out, "paths_inbound_bytes", ta, int64(i.InboundBytes))
					metric(&out, "paths_outbound_bytes", ta, int64(i.OutboundBytes))
					metric(&out, "paths_inbound_frames_in_error", ta, int64(i.InboundFramesInError))
					// deprecated
					metric(&out, "paths_bytes_received", ta, int64(i.BytesReceived))
					metric(&out, "paths_bytes_sent", ta, int64(i.BytesSent))
					metric(&out, "paths_readers", ta, int64(len(i.Readers)))
				}
			}
		} else if pathFilter == "" {
			metric(&out, "paths", "", 0)
			metric(&out, "paths_inbound_bytes", "", 0)
			metric(&out, "paths_outbound_bytes", "", 0)
			metric(&out, "paths_inbound_frames_in_error", "", 0)
			// deprecated
			metric(&out, "paths_bytes_received", "", 0)
			metric(&out, "paths_bytes_sent", "", 0)
			metric(&out, "paths_readers", "", 0)
		}
	}

	if !interfaceIsEmpty(m.hlsServer) &&
		(typ == "" || typ == "hls_muxers") &&
		(!anyFilterActive || hlsMuxerFilter != "") {
		var data *defs.APIHLSMuxerList
		data, err := m.hlsServer.APIMuxersList()
		if err == nil && len(data.Items) != 0 {
			for _, i := range data.Items {
				if hlsMuxerFilter == "" || hlsMuxerFilter == i.Path {
					ta := tags(map[string]string{
						"name": i.Path,
					})
					metric(&out, "hls_muxers", ta, 1)
					metric(&out, "hls_muxers_outbound_bytes", ta, int64(i.OutboundBytes))
					metric(&out, "hls_muxers_outbound_frames_discarded", ta, int64(i.OutboundFramesDiscarded))
					// deprecated
					metric(&out, "hls_muxers_bytes_sent", ta, int64(i.BytesSent))
				}
			}
		} else if hlsMuxerFilter == "" {
			metric(&out, "hls_muxers", "", 0)
			metric(&out, "hls_muxers_outbound_bytes", "", 0)
			metric(&out, "hls_muxers_outbound_frames_discarded", "", 0)
			// deprecated
			metric(&out, "hls_muxers_bytes_sent", "", 0)
		}
	}

	if !interfaceIsEmpty(m.rtspServer) { //nolint:dupl
		if (typ == "" || typ == "rtsp_conns") && (!anyFilterActive || rtspConnFilter != "") {
			var data *defs.APIRTSPConnsList
			data, err := m.rtspServer.APIConnsList()
			if err == nil && len(data.Items) != 0 {
				for _, i := range data.Items {
					if rtspConnFilter == "" || rtspConnFilter == i.ID.String() {
						ta := tags(map[string]string{
							"id": i.ID.String(),
						})
						metric(&out, "rtsp_conns", ta, 1)
						metric(&out, "rtsp_conns_inbound_bytes", ta, int64(i.InboundBytes))
						metric(&out, "rtsp_conns_outbound_bytes", ta, int64(i.OutboundBytes))
						// deprecated
						metric(&out, "rtsp_conns_bytes_received", ta, int64(i.BytesReceived))
						metric(&out, "rtsp_conns_bytes_sent", ta, int64(i.BytesSent))
					}
				}
			} else if rtspConnFilter == "" {
				metric(&out, "rtsp_conns", "", 0)
				metric(&out, "rtsp_conns_inbound_bytes", "", 0)
				metric(&out, "rtsp_conns_outbound_bytes", "", 0)
				// deprecated
				metric(&out, "rtsp_conns_bytes_received", "", 0)
				metric(&out, "rtsp_conns_bytes_sent", "", 0)
			}
		}

		if (typ == "" || typ == "rtsp_sessions") && (!anyFilterActive || rtspSessionFilter != "") {
			var data *defs.APIRTSPSessionList
			data, err := m.rtspServer.APISessionsList()
			if err == nil && len(data.Items) != 0 {
				for _, i := range data.Items {
					if rtspSessionFilter == "" || rtspSessionFilter == i.ID.String() {
						ta := tags(map[string]string{
							"id":         i.ID.String(),
							"state":      string(i.State),
							"path":       i.Path,
							"remoteAddr": i.RemoteAddr,
						})
						metric(&out, "rtsp_sessions", ta, 1)
						metric(&out, "rtsp_sessions_inbound_bytes", ta, int64(i.InboundBytes))
						metric(&out, "rtsp_sessions_inbound_rtp_packets", ta, int64(i.InboundRTPPackets))
						metric(&out, "rtsp_sessions_inbound_rtp_packets_lost", ta, int64(i.InboundRTPPacketsLost))
						metric(&out, "rtsp_sessions_inbound_rtp_packets_in_error", ta, int64(i.InboundRTPPacketsInError))
						metricFloat(&out, "rtsp_sessions_inbound_rtp_packets_jitter", ta, i.InboundRTPPacketsJitter)
						metric(&out, "rtsp_sessions_inbound_rtcp_packets", ta, int64(i.InboundRTCPPackets))
						metric(&out, "rtsp_sessions_inbound_rtcp_packets_in_error", ta, int64(i.InboundRTCPPacketsInError))
						metric(&out, "rtsp_sessions_outbound_bytes", ta, int64(i.OutboundBytes))
						metric(&out, "rtsp_sessions_outbound_rtp_packets", ta, int64(i.OutboundRTPPackets))
						metric(&out, "rtsp_sessions_outbound_rtp_packets_reported_lost", ta, int64(i.OutboundRTPPacketsReportedLost))
						metric(&out, "rtsp_sessions_outbound_rtp_packets_discarded", ta, int64(i.OutboundRTPPacketsDiscarded))
						metric(&out, "rtsp_sessions_outbound_rtcp_packets", ta, int64(i.OutboundRTCPPackets))
						// deprecated
						metric(&out, "rtsp_sessions_bytes_received", ta, int64(i.BytesReceived))
						metric(&out, "rtsp_sessions_bytes_sent", ta, int64(i.BytesSent))
						metric(&out, "rtsp_sessions_rtp_packets_received", ta, int64(i.RTPPacketsReceived))
						metric(&out, "rtsp_sessions_rtp_packets_sent", ta, int64(i.RTPPacketsSent))
						metric(&out, "rtsp_sessions_rtp_packets_lost", ta, int64(i.RTPPacketsLost))
						metric(&out, "rtsp_sessions_rtp_packets_in_error", ta, int64(i.RTPPacketsInError))
						metricFloat(&out, "rtsp_sessions_rtp_packets_jitter", ta, i.RTPPacketsJitter)
						metric(&out, "rtsp_sessions_rtcp_packets_received", ta, int64(i.RTCPPacketsReceived))
						metric(&out, "rtsp_sessions_rtcp_packets_sent", ta, int64(i.RTCPPacketsSent))
						metric(&out, "rtsp_sessions_rtcp_packets_in_error", ta, int64(i.RTCPPacketsInError))
					}
				}
			} else if rtspSessionFilter == "" {
				metric(&out, "rtsp_sessions", "", 0)
				metric(&out, "rtsp_sessions_inbound_bytes", "", 0)
				metric(&out, "rtsp_sessions_inbound_rtp_packets", "", 0)
				metric(&out, "rtsp_sessions_inbound_rtp_packets_lost", "", 0)
				metric(&out, "rtsp_sessions_inbound_rtp_packets_in_error", "", 0)
				metricFloat(&out, "rtsp_sessions_inbound_rtp_packets_jitter", "", 0)
				metric(&out, "rtsp_sessions_inbound_rtcp_packets", "", 0)
				metric(&out, "rtsp_sessions_inbound_rtcp_packets_in_error", "", 0)
				metric(&out, "rtsp_sessions_outbound_bytes", "", 0)
				metric(&out, "rtsp_sessions_outbound_rtp_packets", "", 0)
				metric(&out, "rtsp_sessions_outbound_rtp_packets_reported_lost", "", 0)
				metric(&out, "rtsp_sessions_outbound_rtp_packets_discarded", "", 0)
				metric(&out, "rtsp_sessions_outbound_rtcp_packets", "", 0)
				// deprecated
				metric(&out, "rtsp_sessions_bytes_received", "", 0)
				metric(&out, "rtsp_sessions_bytes_sent", "", 0)
				metric(&out, "rtsp_sessions_rtp_packets_received", "", 0)
				metric(&out, "rtsp_sessions_rtp_packets_sent", "", 0)
				metric(&out, "rtsp_sessions_rtp_packets_lost", "", 0)
				metric(&out, "rtsp_sessions_rtp_packets_in_error", "", 0)
				metricFloat(&out, "rtsp_sessions_rtp_packets_jitter", "", 0)
				metric(&out, "rtsp_sessions_rtcp_packets_received", "", 0)
				metric(&out, "rtsp_sessions_rtcp_packets_sent", "", 0)
				metric(&out, "rtsp_sessions_rtcp_packets_in_error", "", 0)
			}
		}
	}

	if !interfaceIsEmpty(m.rtspsServer) { //nolint:dupl
		if (typ == "" || typ == "rtsps_conns") && (!anyFilterActive || rtspsConnFilter != "") {
			var data *defs.APIRTSPConnsList
			data, err := m.rtspsServer.APIConnsList()
			if err == nil && len(data.Items) != 0 {
				for _, i := range data.Items {
					if rtspsConnFilter == "" || rtspsConnFilter == i.ID.String() {
						ta := tags(map[string]string{
							"id": i.ID.String(),
						})
						metric(&out, "rtsps_conns", ta, 1)
						metric(&out, "rtsps_conns_inbound_bytes", ta, int64(i.InboundBytes))
						metric(&out, "rtsps_conns_outbound_bytes", ta, int64(i.OutboundBytes))
						// deprecated
						metric(&out, "rtsps_conns_bytes_received", ta, int64(i.BytesReceived))
						metric(&out, "rtsps_conns_bytes_sent", ta, int64(i.BytesSent))
					}
				}
			} else if rtspsConnFilter == "" {
				metric(&out, "rtsps_conns", "", 0)
				metric(&out, "rtsps_conns_inbound_bytes", "", 0)
				metric(&out, "rtsps_conns_outbound_bytes", "", 0)
				// deprecated
				metric(&out, "rtsps_conns_bytes_received", "", 0)
				metric(&out, "rtsps_conns_bytes_sent", "", 0)
			}
		}

		if (typ == "" || typ == "rtsps_sessions") && (!anyFilterActive || rtspsSessionFilter != "") {
			var data *defs.APIRTSPSessionList
			data, err := m.rtspsServer.APISessionsList()
			if err == nil && len(data.Items) != 0 {
				for _, i := range data.Items {
					if rtspsSessionFilter == "" || rtspsSessionFilter == i.ID.String() {
						ta := tags(map[string]string{
							"id":         i.ID.String(),
							"state":      string(i.State),
							"path":       i.Path,
							"remoteAddr": i.RemoteAddr,
						})
						metric(&out, "rtsps_sessions", ta, 1)
						metric(&out, "rtsps_sessions_inbound_bytes", ta, int64(i.InboundBytes))
						metric(&out, "rtsps_sessions_inbound_rtp_packets", ta, int64(i.InboundRTPPackets))
						metric(&out, "rtsps_sessions_inbound_rtp_packets_lost", ta, int64(i.InboundRTPPacketsLost))
						metric(&out, "rtsps_sessions_inbound_rtp_packets_in_error", ta, int64(i.InboundRTPPacketsInError))
						metricFloat(&out, "rtsps_sessions_inbound_rtp_packets_jitter", ta, i.InboundRTPPacketsJitter)
						metric(&out, "rtsps_sessions_inbound_rtcp_packets", ta, int64(i.InboundRTCPPackets))
						metric(&out, "rtsps_sessions_inbound_rtcp_packets_in_error", ta, int64(i.InboundRTCPPacketsInError))
						metric(&out, "rtsps_sessions_outbound_bytes", ta, int64(i.OutboundBytes))
						metric(&out, "rtsps_sessions_outbound_rtp_packets", ta, int64(i.OutboundRTPPackets))
						metric(&out, "rtsps_sessions_outbound_rtp_packets_reported_lost", ta, int64(i.OutboundRTPPacketsReportedLost))
						metric(&out, "rtsps_sessions_outbound_rtp_packets_discarded", ta, int64(i.OutboundRTPPacketsDiscarded))
						metric(&out, "rtsps_sessions_outbound_rtcp_packets", ta, int64(i.OutboundRTCPPackets))
						// deprecated
						metric(&out, "rtsps_sessions_bytes_received", ta, int64(i.BytesReceived))
						metric(&out, "rtsps_sessions_bytes_sent", ta, int64(i.BytesSent))
						metric(&out, "rtsps_sessions_rtp_packets_received", ta, int64(i.RTPPacketsReceived))
						metric(&out, "rtsps_sessions_rtp_packets_sent", ta, int64(i.RTPPacketsSent))
						metric(&out, "rtsps_sessions_rtp_packets_lost", ta, int64(i.RTPPacketsLost))
						metric(&out, "rtsps_sessions_rtp_packets_in_error", ta, int64(i.RTPPacketsInError))
						metricFloat(&out, "rtsps_sessions_rtp_packets_jitter", ta, i.RTPPacketsJitter)
						metric(&out, "rtsps_sessions_rtcp_packets_received", ta, int64(i.RTCPPacketsReceived))
						metric(&out, "rtsps_sessions_rtcp_packets_sent", ta, int64(i.RTCPPacketsSent))
						metric(&out, "rtsps_sessions_rtcp_packets_in_error", ta, int64(i.RTCPPacketsInError))
					}
				}
			} else if rtspsSessionFilter == "" {
				metric(&out, "rtsps_sessions", "", 0)
				metric(&out, "rtsps_sessions_inbound_bytes", "", 0)
				metric(&out, "rtsps_sessions_inbound_rtp_packets", "", 0)
				metric(&out, "rtsps_sessions_inbound_rtp_packets_lost", "", 0)
				metric(&out, "rtsps_sessions_inbound_rtp_packets_in_error", "", 0)
				metricFloat(&out, "rtsps_sessions_inbound_rtp_packets_jitter", "", 0)
				metric(&out, "rtsps_sessions_inbound_rtcp_packets", "", 0)
				metric(&out, "rtsps_sessions_inbound_rtcp_packets_in_error", "", 0)
				metric(&out, "rtsps_sessions_outbound_bytes", "", 0)
				metric(&out, "rtsps_sessions_outbound_rtp_packets", "", 0)
				metric(&out, "rtsps_sessions_outbound_rtp_packets_reported_lost", "", 0)
				metric(&out, "rtsps_sessions_outbound_rtp_packets_discarded", "", 0)
				metric(&out, "rtsps_sessions_outbound_rtcp_packets", "", 0)
				// deprecated
				metric(&out, "rtsps_sessions_bytes_received", "", 0)
				metric(&out, "rtsps_sessions_bytes_sent", "", 0)
				metric(&out, "rtsps_sessions_rtp_packets_received", "", 0)
				metric(&out, "rtsps_sessions_rtp_packets_sent", "", 0)
				metric(&out, "rtsps_sessions_rtp_packets_lost", "", 0)
				metric(&out, "rtsps_sessions_rtp_packets_in_error", "", 0)
				metricFloat(&out, "rtsps_sessions_rtp_packets_jitter", "", 0)
				metric(&out, "rtsps_sessions_rtcp_packets_received", "", 0)
				metric(&out, "rtsps_sessions_rtcp_packets_sent", "", 0)
				metric(&out, "rtsps_sessions_rtcp_packets_in_error", "", 0)
			}
		}
	}

	if !interfaceIsEmpty(m.rtmpServer) && //nolint:dupl
		(typ == "" || typ == "rtmp_conns") &&
		(!anyFilterActive || rtmpConnFilter != "") {
		var data *defs.APIRTMPConnList
		data, err := m.rtmpServer.APIConnsList()
		if err == nil && len(data.Items) != 0 {
			for _, i := range data.Items {
				if rtmpConnFilter == "" || rtmpConnFilter == i.ID.String() {
					ta := tags(map[string]string{
						"id":         i.ID.String(),
						"state":      string(i.State),
						"path":       i.Path,
						"remoteAddr": i.RemoteAddr,
					})
					metric(&out, "rtmp_conns", ta, 1)
					metric(&out, "rtmp_conns_inbound_bytes", ta, int64(i.InboundBytes))
					metric(&out, "rtmp_conns_outbound_bytes", ta, int64(i.OutboundBytes))
					metric(&out, "rtmp_conns_outbound_frames_discarded", ta, int64(i.OutboundFramesDiscarded))
					// deprecated
					metric(&out, "rtmp_conns_bytes_received", ta, int64(i.BytesReceived))
					metric(&out, "rtmp_conns_bytes_sent", ta, int64(i.BytesSent))
				}
			}
		} else if rtmpConnFilter == "" {
			metric(&out, "rtmp_conns", "", 0)
			metric(&out, "rtmp_conns_inbound_bytes", "", 0)
			metric(&out, "rtmp_conns_outbound_bytes", "", 0)
			metric(&out, "rtmp_conns_outbound_frames_discarded", "", 0)
			// deprecated
			metric(&out, "rtmp_conns_bytes_received", "", 0)
			metric(&out, "rtmp_conns_bytes_sent", "", 0)
		}
	}

	if !interfaceIsEmpty(m.rtmpsServer) && //nolint:dupl
		(typ == "" || typ == "rtmp_conns") &&
		(!anyFilterActive || rtmpsConnFilter != "") {
		var data *defs.APIRTMPConnList
		data, err := m.rtmpsServer.APIConnsList()
		if err == nil && len(data.Items) != 0 {
			for _, i := range data.Items {
				if rtmpsConnFilter == "" || rtmpsConnFilter == i.ID.String() {
					ta := tags(map[string]string{
						"id":         i.ID.String(),
						"state":      string(i.State),
						"path":       i.Path,
						"remoteAddr": i.RemoteAddr,
					})
					metric(&out, "rtmps_conns", ta, 1)
					metric(&out, "rtmps_conns_inbound_bytes", ta, int64(i.InboundBytes))
					metric(&out, "rtmps_conns_outbound_bytes", ta, int64(i.OutboundBytes))
					metric(&out, "rtmps_conns_outbound_frames_discarded", ta, int64(i.OutboundFramesDiscarded))
					// deprecated
					metric(&out, "rtmps_conns_bytes_received", ta, int64(i.BytesReceived))
					metric(&out, "rtmps_conns_bytes_sent", ta, int64(i.BytesSent))
				}
			}
		} else if rtmpsConnFilter == "" {
			metric(&out, "rtmps_conns", "", 0)
			metric(&out, "rtmps_conns_inbound_bytes", "", 0)
			metric(&out, "rtmps_conns_outbound_bytes", "", 0)
			metric(&out, "rtmps_conns_outbound_frames_discarded", "", 0)
			// deprecated
			metric(&out, "rtmps_conns_bytes_received", "", 0)
			metric(&out, "rtmps_conns_bytes_sent", "", 0)
		}
	}

	if !interfaceIsEmpty(m.srtServer) &&
		(typ == "" || typ == "srt_conns") &&
		(!anyFilterActive || srtConnFilter != "") {
		var data *defs.APISRTConnList
		data, err := m.srtServer.APIConnsList()
		if err == nil && len(data.Items) != 0 {
			for _, i := range data.Items {
				if srtConnFilter == "" || srtConnFilter == i.ID.String() {
					ta := tags(map[string]string{
						"id":         i.ID.String(),
						"state":      string(i.State),
						"path":       i.Path,
						"remoteAddr": i.RemoteAddr,
					})
					metric(&out, "srt_conns", ta, 1)
					metric(&out, "srt_conns_packets_sent", ta, int64(i.PacketsSent))
					metric(&out, "srt_conns_packets_received", ta, int64(i.PacketsReceived))
					metric(&out, "srt_conns_packets_sent_unique", ta, int64(i.PacketsSentUnique))
					metric(&out, "srt_conns_packets_received_unique", ta, int64(i.PacketsReceivedUnique))
					metric(&out, "srt_conns_packets_send_loss", ta, int64(i.PacketsSendLoss))
					metric(&out, "srt_conns_packets_received_loss", ta, int64(i.PacketsReceivedLoss))
					metric(&out, "srt_conns_packets_retrans", ta, int64(i.PacketsRetrans))
					metric(&out, "srt_conns_packets_received_retrans", ta, int64(i.PacketsReceivedRetrans))
					metric(&out, "srt_conns_packets_sent_ack", ta, int64(i.PacketsSentACK))
					metric(&out, "srt_conns_packets_received_ack", ta, int64(i.PacketsReceivedACK))
					metric(&out, "srt_conns_packets_sent_nak", ta, int64(i.PacketsSentNAK))
					metric(&out, "srt_conns_packets_received_nak", ta, int64(i.PacketsReceivedNAK))
					metric(&out, "srt_conns_packets_sent_km", ta, int64(i.PacketsSentKM))
					metric(&out, "srt_conns_packets_received_km", ta, int64(i.PacketsReceivedKM))
					metric(&out, "srt_conns_us_snd_duration", ta, int64(i.UsSndDuration))
					metric(&out, "srt_conns_packets_received_belated", ta, int64(i.PacketsReceivedBelated))
					metric(&out, "srt_conns_packets_send_drop", ta, int64(i.PacketsSendDrop))
					metric(&out, "srt_conns_packets_received_drop", ta, int64(i.PacketsReceivedDrop))
					metric(&out, "srt_conns_packets_received_undecrypt", ta, int64(i.PacketsReceivedUndecrypt))
					metric(&out, "srt_conns_bytes_sent", ta, int64(i.BytesSent))
					metric(&out, "srt_conns_bytes_received", ta, int64(i.BytesReceived))
					metric(&out, "srt_conns_bytes_sent_unique", ta, int64(i.BytesSentUnique))
					metric(&out, "srt_conns_bytes_received_unique", ta, int64(i.BytesReceivedUnique))
					metric(&out, "srt_conns_bytes_received_loss", ta, int64(i.BytesReceivedLoss))
					metric(&out, "srt_conns_bytes_retrans", ta, int64(i.BytesRetrans))
					metric(&out, "srt_conns_bytes_received_retrans", ta, int64(i.BytesReceivedRetrans))
					metric(&out, "srt_conns_bytes_received_belated", ta, int64(i.BytesReceivedBelated))
					metric(&out, "srt_conns_bytes_send_drop", ta, int64(i.BytesSendDrop))
					metric(&out, "srt_conns_bytes_received_drop", ta, int64(i.BytesReceivedDrop))
					metric(&out, "srt_conns_bytes_received_undecrypt", ta, int64(i.BytesReceivedUndecrypt))
					metricFloat(&out, "srt_conns_us_packets_send_period", ta, i.UsPacketsSendPeriod)
					metric(&out, "srt_conns_packets_flow_window", ta, int64(i.PacketsFlowWindow))
					metric(&out, "srt_conns_packets_flight_size", ta, int64(i.PacketsFlightSize))
					metricFloat(&out, "srt_conns_ms_rtt", ta, i.MsRTT)
					metricFloat(&out, "srt_conns_mbps_send_rate", ta, i.MbpsSendRate)
					metricFloat(&out, "srt_conns_mbps_receive_rate", ta, i.MbpsReceiveRate)
					metricFloat(&out, "srt_conns_mbps_link_capacity", ta, i.MbpsLinkCapacity)
					metric(&out, "srt_conns_bytes_avail_send_buf", ta, int64(i.BytesAvailSendBuf))
					metric(&out, "srt_conns_bytes_avail_receive_buf", ta, int64(i.BytesAvailReceiveBuf))
					metricFloat(&out, "srt_conns_mbps_max_bw", ta, i.MbpsMaxBW)
					metric(&out, "srt_conns_bytes_mss", ta, int64(i.ByteMSS))
					metric(&out, "srt_conns_packets_send_buf", ta, int64(i.PacketsSendBuf))
					metric(&out, "srt_conns_bytes_send_buf", ta, int64(i.BytesSendBuf))
					metric(&out, "srt_conns_ms_send_buf", ta, int64(i.MsSendBuf))
					metric(&out, "srt_conns_ms_send_tsb_pd_delay", ta, int64(i.MsSendTsbPdDelay))
					metric(&out, "srt_conns_packets_receive_buf", ta, int64(i.PacketsReceiveBuf))
					metric(&out, "srt_conns_bytes_receive_buf", ta, int64(i.BytesReceiveBuf))
					metric(&out, "srt_conns_ms_receive_buf", ta, int64(i.MsReceiveBuf))
					metric(&out, "srt_conns_ms_receive_tsb_pd_delay", ta, int64(i.MsReceiveTsbPdDelay))
					metric(&out, "srt_conns_packets_reorder_tolerance", ta, int64(i.PacketsReorderTolerance))
					metric(&out, "srt_conns_packets_received_avg_belated_time", ta, int64(i.PacketsReceivedAvgBelatedTime))
					metricFloat(&out, "srt_conns_packets_send_loss_rate", ta, i.PacketsSendLossRate)
					metricFloat(&out, "srt_conns_packets_received_loss_rate", ta, i.PacketsReceivedLossRate)
					metric(&out, "srt_conns_outbound_frames_discarded", ta, int64(i.OutboundFramesDiscarded))
				}
			}
		} else if srtConnFilter == "" {
			metric(&out, "srt_conns", "", 0)
			metric(&out, "srt_conns_packets_sent", "", 0)
			metric(&out, "srt_conns_packets_received", "", 0)
			metric(&out, "srt_conns_packets_sent_unique", "", 0)
			metric(&out, "srt_conns_packets_received_unique", "", 0)
			metric(&out, "srt_conns_packets_send_loss", "", 0)
			metric(&out, "srt_conns_packets_received_loss", "", 0)
			metric(&out, "srt_conns_packets_retrans", "", 0)
			metric(&out, "srt_conns_packets_received_retrans", "", 0)
			metric(&out, "srt_conns_packets_sent_ack", "", 0)
			metric(&out, "srt_conns_packets_received_ack", "", 0)
			metric(&out, "srt_conns_packets_sent_nak", "", 0)
			metric(&out, "srt_conns_packets_received_nak", "", 0)
			metric(&out, "srt_conns_packets_sent_km", "", 0)
			metric(&out, "srt_conns_packets_received_km", "", 0)
			metric(&out, "srt_conns_us_snd_duration", "", 0)
			metric(&out, "srt_conns_packets_received_belated", "", 0)
			metric(&out, "srt_conns_packets_send_drop", "", 0)
			metric(&out, "srt_conns_packets_received_drop", "", 0)
			metric(&out, "srt_conns_packets_received_undecrypt", "", 0)
			metric(&out, "srt_conns_bytes_sent", "", 0)
			metric(&out, "srt_conns_bytes_received", "", 0)
			metric(&out, "srt_conns_bytes_sent_unique", "", 0)
			metric(&out, "srt_conns_bytes_received_unique", "", 0)
			metric(&out, "srt_conns_bytes_received_loss", "", 0)
			metric(&out, "srt_conns_bytes_retrans", "", 0)
			metric(&out, "srt_conns_bytes_received_retrans", "", 0)
			metric(&out, "srt_conns_bytes_received_belated", "", 0)
			metric(&out, "srt_conns_bytes_send_drop", "", 0)
			metric(&out, "srt_conns_bytes_received_drop", "", 0)
			metric(&out, "srt_conns_bytes_received_undecrypt", "", 0)
			metricFloat(&out, "srt_conns_us_packets_send_period", "", 0)
			metric(&out, "srt_conns_packets_flow_window", "", 0)
			metric(&out, "srt_conns_packets_flight_size", "", 0)
			metricFloat(&out, "srt_conns_ms_rtt", "", 0)
			metricFloat(&out, "srt_conns_mbps_send_rate", "", 0)
			metricFloat(&out, "srt_conns_mbps_receive_rate", "", 0)
			metricFloat(&out, "srt_conns_mbps_link_capacity", "", 0)
			metric(&out, "srt_conns_bytes_avail_send_buf", "", 0)
			metric(&out, "srt_conns_bytes_avail_receive_buf", "", 0)
			metricFloat(&out, "srt_conns_mbps_max_bw", "", 0)
			metric(&out, "srt_conns_bytes_mss", "", 0)
			metric(&out, "srt_conns_packets_send_buf", "", 0)
			metric(&out, "srt_conns_bytes_send_buf", "", 0)
			metric(&out, "srt_conns_ms_send_buf", "", 0)
			metric(&out, "srt_conns_ms_send_tsb_pd_delay", "", 0)
			metric(&out, "srt_conns_packets_receive_buf", "", 0)
			metric(&out, "srt_conns_bytes_receive_buf", "", 0)
			metric(&out, "srt_conns_ms_receive_buf", "", 0)
			metric(&out, "srt_conns_ms_receive_tsb_pd_delay", "", 0)
			metric(&out, "srt_conns_packets_reorder_tolerance", "", 0)
			metric(&out, "srt_conns_packets_received_avg_belated_time", "", 0)
			metricFloat(&out, "srt_conns_packets_send_loss_rate", "", 0)
			metricFloat(&out, "srt_conns_packets_received_loss_rate", "", 0)
			metric(&out, "srt_conns_outbound_frames_discarded", "", 0)
		}
	}

	if !interfaceIsEmpty(m.webRTCServer) &&
		(typ == "" || typ == "webrtc_sessions") &&
		(!anyFilterActive || webrtcSessionFilter != "") {
		var data *defs.APIWebRTCSessionList
		data, err := m.webRTCServer.APISessionsList()
		if err == nil && len(data.Items) != 0 {
			for _, i := range data.Items {
				if webrtcSessionFilter == "" || webrtcSessionFilter == i.ID.String() {
					ta := tags(map[string]string{
						"id":         i.ID.String(),
						"state":      string(i.State),
						"path":       i.Path,
						"remoteAddr": i.RemoteAddr,
					})
					metric(&out, "webrtc_sessions", ta, 1)
					metric(&out, "webrtc_sessions_inbound_bytes", ta, int64(i.InboundBytes))
					metric(&out, "webrtc_sessions_inbound_rtp_packets", ta, int64(i.InboundRTPPackets))
					metric(&out, "webrtc_sessions_inbound_rtp_packets_lost", ta, int64(i.InboundRTPPacketsLost))
					metricFloat(&out, "webrtc_sessions_inbound_rtp_packets_jitter", ta, i.InboundRTPPacketsJitter)
					metric(&out, "webrtc_sessions_inbound_rtcp_packets", ta, int64(i.InboundRTCPPackets))
					metric(&out, "webrtc_sessions_outbound_bytes", ta, int64(i.OutboundBytes))
					metric(&out, "webrtc_sessions_outbound_rtp_packets", ta, int64(i.OutboundRTPPackets))
					metric(&out, "webrtc_sessions_outbound_rtcp_packets", ta, int64(i.OutboundRTCPPackets))
					metric(&out, "webrtc_sessions_outbound_frames_discarded", ta, int64(i.OutboundFramesDiscarded))
					// deprecated
					metric(&out, "webrtc_sessions_bytes_received", ta, int64(i.BytesReceived))
					metric(&out, "webrtc_sessions_bytes_sent", ta, int64(i.BytesSent))
					metric(&out, "webrtc_sessions_rtp_packets_received", ta, int64(i.RTPPacketsReceived))
					metric(&out, "webrtc_sessions_rtp_packets_sent", ta, int64(i.RTPPacketsSent))
					metric(&out, "webrtc_sessions_rtp_packets_lost", ta, int64(i.RTPPacketsLost))
					metricFloat(&out, "webrtc_sessions_rtp_packets_jitter", ta, i.RTPPacketsJitter)
					metric(&out, "webrtc_sessions_rtcp_packets_received", ta, int64(i.RTCPPacketsReceived))
					metric(&out, "webrtc_sessions_rtcp_packets_sent", ta, int64(i.RTCPPacketsSent))
				}
			}
		} else if webrtcSessionFilter == "" {
			metric(&out, "webrtc_sessions", "", 0)
			metric(&out, "webrtc_sessions_inbound_bytes", "", 0)
			metric(&out, "webrtc_sessions_inbound_rtp_packets", "", 0)
			metric(&out, "webrtc_sessions_inbound_rtp_packets_lost", "", 0)
			metricFloat(&out, "webrtc_sessions_inbound_rtp_packets_jitter", "", 0)
			metric(&out, "webrtc_sessions_inbound_rtcp_packets", "", 0)
			metric(&out, "webrtc_sessions_outbound_bytes", "", 0)
			metric(&out, "webrtc_sessions_outbound_rtp_packets", "", 0)
			metric(&out, "webrtc_sessions_outbound_rtcp_packets", "", 0)
			metric(&out, "webrtc_sessions_outbound_frames_discarded", "", 0)
			// deprecated
			metric(&out, "webrtc_sessions_bytes_received", "", 0)
			metric(&out, "webrtc_sessions_bytes_sent", "", 0)
			metric(&out, "webrtc_sessions_rtp_packets_received", "", 0)
			metric(&out, "webrtc_sessions_rtp_packets_sent", "", 0)
			metric(&out, "webrtc_sessions_rtp_packets_lost", "", 0)
			metricFloat(&out, "webrtc_sessions_rtp_packets_jitter", "", 0)
			metric(&out, "webrtc_sessions_rtcp_packets_received", "", 0)
			metric(&out, "webrtc_sessions_rtcp_packets_sent", "", 0)
		}
	}

	ctx.Writer.WriteHeader(http.StatusOK)
	ctx.Writer.WriteString(out.String()) //nolint:errcheck
}

// SetPathManager is called by core.
func (m *Metrics) SetPathManager(s defs.APIPathManager) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.pathManager = s
}

// SetHLSServer is called by core.
func (m *Metrics) SetHLSServer(s defs.APIHLSServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.hlsServer = s
}

// SetRTSPServer is called by core.
func (m *Metrics) SetRTSPServer(s defs.APIRTSPServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.rtspServer = s
}

// SetRTSPSServer is called by core.
func (m *Metrics) SetRTSPSServer(s defs.APIRTSPServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.rtspsServer = s
}

// SetRTMPServer is called by core.
func (m *Metrics) SetRTMPServer(s defs.APIRTMPServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.rtmpServer = s
}

// SetRTMPSServer is called by core.
func (m *Metrics) SetRTMPSServer(s defs.APIRTMPServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.rtmpsServer = s
}

// SetSRTServer is called by core.
func (m *Metrics) SetSRTServer(s defs.APISRTServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.srtServer = s
}

// SetWebRTCServer is called by core.
func (m *Metrics) SetWebRTCServer(s defs.APIWebRTCServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.webRTCServer = s
}
