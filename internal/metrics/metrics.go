// Package metrics contains the metrics provider.
package metrics

import (
	"io"
	"net"
	"net/http"
	"reflect"
	"sort"
	"strconv"
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
	o := "{"

	first := true
	for _, k := range sortedKeys(m) {
		if first {
			first = false
		} else {
			o += ","
		}
		o += k + "=\"" + m[k] + "\""
	}

	o += "}"
	return o
}

func metric(key string, tags string, value int64) string {
	return key + tags + " " + strconv.FormatInt(value, 10) + "\n"
}

func metricFloat(key string, tags string, value float64) string {
	return key + tags + " " + strconv.FormatFloat(value, 'f', -1, 64) + "\n"
}

type metricsAuthManager interface {
	Authenticate(req *auth.Request) *auth.Error
}

type metricsParent interface {
	logger.Writer
}

// Metrics is a metrics provider.
type Metrics struct {
	Address        string
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
		Address:      m.Address,
		AllowOrigins: m.AllowOrigins,
		ReadTimeout:  time.Duration(m.ReadTimeout),
		WriteTimeout: time.Duration(m.WriteTimeout),
		Encryption:   m.Encryption,
		ServerCert:   m.ServerCert,
		ServerKey:    m.ServerKey,
		Handler:      router,
		Parent:       m,
	}
	err := m.httpServer.Initialize()
	if err != nil {
		return err
	}

	m.Log(logger.Info, "listener opened on "+m.Address)

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

func (m *Metrics) middlewareAuth(ctx *gin.Context) {
	req := &auth.Request{
		Action:      conf.AuthActionMetrics,
		Query:       ctx.Request.URL.RawQuery,
		Credentials: httpp.Credentials(ctx.Request),
		IP:          net.ParseIP(ctx.ClientIP()),
	}

	err := m.AuthManager.Authenticate(req)
	if err != nil {
		if err.AskCredentials {
			ctx.Header("WWW-Authenticate", `Basic realm="mediamtx"`)
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, &defs.APIError{
				Status: "error",
				Error:  "authentication error",
			})
			return
		}

		m.Log(logger.Info, "connection %v failed to authenticate: %v", httpp.RemoteAddr(ctx), err.Wrapped)

		// wait some seconds to delay brute force attacks
		<-time.After(auth.PauseAfterError)

		ctx.AbortWithStatusJSON(http.StatusUnauthorized, &defs.APIError{
			Status: "error",
			Error:  "authentication error",
		})
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

	out := ""

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
					out += metric("paths", ta, 1)
					out += metric("paths_bytes_received", ta, int64(i.BytesReceived))
					out += metric("paths_bytes_sent", ta, int64(i.BytesSent))
					out += metric("paths_readers", ta, int64(len(i.Readers)))
				}
			}
		} else if pathFilter == "" {
			out += metric("paths", "", 0)
			out += metric("paths_bytes_received", "", 0)
			out += metric("paths_bytes_sent", "", 0)
			out += metric("paths_readers", "", 0)
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
					out += metric("hls_muxers", ta, 1)
					out += metric("hls_muxers_bytes_sent", ta, int64(i.BytesSent))
				}
			}
		} else if hlsMuxerFilter == "" {
			out += metric("hls_muxers", "", 0)
			out += metric("hls_muxers_bytes_sent", "", 0)
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
						out += metric("rtsp_conns", ta, 1)
						out += metric("rtsp_conns_bytes_received", ta, int64(i.BytesReceived))
						out += metric("rtsp_conns_bytes_sent", ta, int64(i.BytesSent))
					}
				}
			} else if rtspConnFilter == "" {
				out += metric("rtsp_conns", "", 0)
				out += metric("rtsp_conns_bytes_received", "", 0)
				out += metric("rtsp_conns_bytes_sent", "", 0)
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
						out += metric("rtsp_sessions", ta, 1)
						out += metric("rtsp_sessions_bytes_received", ta, int64(i.BytesReceived))
						out += metric("rtsp_sessions_bytes_sent", ta, int64(i.BytesSent))
						out += metric("rtsp_sessions_rtp_packets_received", ta, int64(i.RTPPacketsReceived))
						out += metric("rtsp_sessions_rtp_packets_sent", ta, int64(i.RTPPacketsSent))
						out += metric("rtsp_sessions_rtp_packets_lost", ta, int64(i.RTPPacketsLost))
						out += metric("rtsp_sessions_rtp_packets_in_error", ta, int64(i.RTPPacketsInError))
						out += metricFloat("rtsp_sessions_rtp_packets_jitter", ta, i.RTPPacketsJitter)
						out += metric("rtsp_sessions_rtcp_packets_received", ta, int64(i.RTCPPacketsReceived))
						out += metric("rtsp_sessions_rtcp_packets_sent", ta, int64(i.RTCPPacketsSent))
						out += metric("rtsp_sessions_rtcp_packets_in_error", ta, int64(i.RTCPPacketsInError))
					}
				}
			} else if rtspSessionFilter == "" {
				out += metric("rtsp_sessions", "", 0)
				out += metric("rtsp_sessions_bytes_received", "", 0)
				out += metric("rtsp_sessions_bytes_sent", "", 0)
				out += metric("rtsp_sessions_rtp_packets_received", "", 0)
				out += metric("rtsp_sessions_rtp_packets_sent", "", 0)
				out += metric("rtsp_sessions_rtp_packets_lost", "", 0)
				out += metric("rtsp_sessions_rtp_packets_in_error", "", 0)
				out += metricFloat("rtsp_sessions_rtp_packets_jitter", "", 0)
				out += metric("rtsp_sessions_rtcp_packets_received", "", 0)
				out += metric("rtsp_sessions_rtcp_packets_sent", "", 0)
				out += metric("rtsp_sessions_rtcp_packets_in_error", "", 0)
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
						out += metric("rtsps_conns", ta, 1)
						out += metric("rtsps_conns_bytes_received", ta, int64(i.BytesReceived))
						out += metric("rtsps_conns_bytes_sent", ta, int64(i.BytesSent))
					}
				}
			} else if rtspsConnFilter == "" {
				out += metric("rtsps_conns", "", 0)
				out += metric("rtsps_conns_bytes_received", "", 0)
				out += metric("rtsps_conns_bytes_sent", "", 0)
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
						out += metric("rtsps_sessions", ta, 1)
						out += metric("rtsps_sessions_bytes_received", ta, int64(i.BytesReceived))
						out += metric("rtsps_sessions_bytes_sent", ta, int64(i.BytesSent))
						out += metric("rtsps_sessions_rtp_packets_received", ta, int64(i.RTPPacketsReceived))
						out += metric("rtsps_sessions_rtp_packets_sent", ta, int64(i.RTPPacketsSent))
						out += metric("rtsps_sessions_rtp_packets_lost", ta, int64(i.RTPPacketsLost))
						out += metric("rtsps_sessions_rtp_packets_in_error", ta, int64(i.RTPPacketsInError))
						out += metricFloat("rtsps_sessions_rtp_packets_jitter", ta, i.RTPPacketsJitter)
						out += metric("rtsps_sessions_rtcp_packets_received", ta, int64(i.RTCPPacketsReceived))
						out += metric("rtsps_sessions_rtcp_packets_sent", ta, int64(i.RTCPPacketsSent))
						out += metric("rtsps_sessions_rtcp_packets_in_error", ta, int64(i.RTCPPacketsInError))
					}
				}
			} else if rtspsSessionFilter == "" {
				out += metric("rtsps_sessions", "", 0)
				out += metric("rtsps_sessions_bytes_received", "", 0)
				out += metric("rtsps_sessions_bytes_sent", "", 0)
				out += metric("rtsps_sessions_rtp_packets_received", "", 0)
				out += metric("rtsps_sessions_rtp_packets_sent", "", 0)
				out += metric("rtsps_sessions_rtp_packets_lost", "", 0)
				out += metric("rtsps_sessions_rtp_packets_in_error", "", 0)
				out += metricFloat("rtsps_sessions_rtp_packets_jitter", "", 0)
				out += metric("rtsps_sessions_rtcp_packets_received", "", 0)
				out += metric("rtsps_sessions_rtcp_packets_sent", "", 0)
				out += metric("rtsps_sessions_rtcp_packets_in_error", "", 0)
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
					out += metric("rtmp_conns", ta, 1)
					out += metric("rtmp_conns_bytes_received", ta, int64(i.BytesReceived))
					out += metric("rtmp_conns_bytes_sent", ta, int64(i.BytesSent))
				}
			}
		} else if rtmpConnFilter == "" {
			out += metric("rtmp_conns", "", 0)
			out += metric("rtmp_conns_bytes_received", "", 0)
			out += metric("rtmp_conns_bytes_sent", "", 0)
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
					out += metric("rtmps_conns", ta, 1)
					out += metric("rtmps_conns_bytes_received", ta, int64(i.BytesReceived))
					out += metric("rtmps_conns_bytes_sent", ta, int64(i.BytesSent))
				}
			}
		} else if rtmpsConnFilter == "" {
			out += metric("rtmps_conns", "", 0)
			out += metric("rtmps_conns_bytes_received", "", 0)
			out += metric("rtmps_conns_bytes_sent", "", 0)
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
					out += metric("srt_conns", ta, 1)
					out += metric("srt_conns_packets_sent", ta, int64(i.PacketsSent))
					out += metric("srt_conns_packets_received", ta, int64(i.PacketsReceived))
					out += metric("srt_conns_packets_sent_unique", ta, int64(i.PacketsSentUnique))
					out += metric("srt_conns_packets_received_unique", ta, int64(i.PacketsReceivedUnique))
					out += metric("srt_conns_packets_send_loss", ta, int64(i.PacketsSendLoss))
					out += metric("srt_conns_packets_received_loss", ta, int64(i.PacketsReceivedLoss))
					out += metric("srt_conns_packets_retrans", ta, int64(i.PacketsRetrans))
					out += metric("srt_conns_packets_received_retrans", ta, int64(i.PacketsReceivedRetrans))
					out += metric("srt_conns_packets_sent_ack", ta, int64(i.PacketsSentACK))
					out += metric("srt_conns_packets_received_ack", ta, int64(i.PacketsReceivedACK))
					out += metric("srt_conns_packets_sent_nak", ta, int64(i.PacketsSentNAK))
					out += metric("srt_conns_packets_received_nak", ta, int64(i.PacketsReceivedNAK))
					out += metric("srt_conns_packets_sent_km", ta, int64(i.PacketsSentKM))
					out += metric("srt_conns_packets_received_km", ta, int64(i.PacketsReceivedKM))
					out += metric("srt_conns_us_snd_duration", ta, int64(i.UsSndDuration))
					out += metric("srt_conns_packets_send_drop", ta, int64(i.PacketsSendDrop))
					out += metric("srt_conns_packets_received_drop", ta, int64(i.PacketsReceivedDrop))
					out += metric("srt_conns_packets_received_undecrypt", ta, int64(i.PacketsReceivedUndecrypt))
					out += metric("srt_conns_bytes_sent", ta, int64(i.BytesSent))
					out += metric("srt_conns_bytes_received", ta, int64(i.BytesReceived))
					out += metric("srt_conns_bytes_sent_unique", ta, int64(i.BytesSentUnique))
					out += metric("srt_conns_bytes_received_unique", ta, int64(i.BytesReceivedUnique))
					out += metric("srt_conns_bytes_received_loss", ta, int64(i.BytesReceivedLoss))
					out += metric("srt_conns_bytes_retrans", ta, int64(i.BytesRetrans))
					out += metric("srt_conns_bytes_received_retrans", ta, int64(i.BytesReceivedRetrans))
					out += metric("srt_conns_bytes_send_drop", ta, int64(i.BytesSendDrop))
					out += metric("srt_conns_bytes_received_drop", ta, int64(i.BytesReceivedDrop))
					out += metric("srt_conns_bytes_received_undecrypt", ta, int64(i.BytesReceivedUndecrypt))
					out += metricFloat("srt_conns_us_packets_send_period", ta, i.UsPacketsSendPeriod)
					out += metric("srt_conns_packets_flow_window", ta, int64(i.PacketsFlowWindow))
					out += metric("srt_conns_packets_flight_size", ta, int64(i.PacketsFlightSize))
					out += metricFloat("srt_conns_ms_rtt", ta, i.MsRTT)
					out += metricFloat("srt_conns_mbps_send_rate", ta, i.MbpsSendRate)
					out += metricFloat("srt_conns_mbps_receive_rate", ta, i.MbpsReceiveRate)
					out += metricFloat("srt_conns_mbps_link_capacity", ta, i.MbpsLinkCapacity)
					out += metric("srt_conns_bytes_avail_send_buf", ta, int64(i.BytesAvailSendBuf))
					out += metric("srt_conns_bytes_avail_receive_buf", ta, int64(i.BytesAvailReceiveBuf))
					out += metricFloat("srt_conns_mbps_max_bw", ta, i.MbpsMaxBW)
					out += metric("srt_conns_bytes_mss", ta, int64(i.ByteMSS))
					out += metric("srt_conns_packets_send_buf", ta, int64(i.PacketsSendBuf))
					out += metric("srt_conns_bytes_send_buf", ta, int64(i.BytesSendBuf))
					out += metric("srt_conns_ms_send_buf", ta, int64(i.MsSendBuf))
					out += metric("srt_conns_ms_send_tsb_pd_delay", ta, int64(i.MsSendTsbPdDelay))
					out += metric("srt_conns_packets_receive_buf", ta, int64(i.PacketsReceiveBuf))
					out += metric("srt_conns_bytes_receive_buf", ta, int64(i.BytesReceiveBuf))
					out += metric("srt_conns_ms_receive_buf", ta, int64(i.MsReceiveBuf))
					out += metric("srt_conns_ms_receive_tsb_pd_delay", ta, int64(i.MsReceiveTsbPdDelay))
					out += metric("srt_conns_packets_reorder_tolerance", ta, int64(i.PacketsReorderTolerance))
					out += metric("srt_conns_packets_received_avg_belated_time", ta, int64(i.PacketsReceivedAvgBelatedTime))
					out += metricFloat("srt_conns_packets_send_loss_rate", ta, i.PacketsSendLossRate)
					out += metricFloat("srt_conns_packets_received_loss_rate", ta, i.PacketsReceivedLossRate)
				}
			}
		} else if srtConnFilter == "" {
			out += metric("srt_conns", "", 0)
			out += metric("srt_conns_packets_sent", "", 0)
			out += metric("srt_conns_packets_received", "", 0)
			out += metric("srt_conns_packets_sent_unique", "", 0)
			out += metric("srt_conns_packets_received_unique", "", 0)
			out += metric("srt_conns_packets_send_loss", "", 0)
			out += metric("srt_conns_packets_received_loss", "", 0)
			out += metric("srt_conns_packets_retrans", "", 0)
			out += metric("srt_conns_packets_received_retrans", "", 0)
			out += metric("srt_conns_packets_sent_ack", "", 0)
			out += metric("srt_conns_packets_received_ack", "", 0)
			out += metric("srt_conns_packets_sent_nak", "", 0)
			out += metric("srt_conns_packets_received_nak", "", 0)
			out += metric("srt_conns_packets_sent_km", "", 0)
			out += metric("srt_conns_packets_received_km", "", 0)
			out += metric("srt_conns_us_snd_duration", "", 0)
			out += metric("srt_conns_packets_send_drop", "", 0)
			out += metric("srt_conns_packets_received_drop", "", 0)
			out += metric("srt_conns_packets_received_undecrypt", "", 0)
			out += metric("srt_conns_bytes_sent", "", 0)
			out += metric("srt_conns_bytes_received", "", 0)
			out += metric("srt_conns_bytes_sent_unique", "", 0)
			out += metric("srt_conns_bytes_received_unique", "", 0)
			out += metric("srt_conns_bytes_received_loss", "", 0)
			out += metric("srt_conns_bytes_retrans", "", 0)
			out += metric("srt_conns_bytes_received_retrans", "", 0)
			out += metric("srt_conns_bytes_send_drop", "", 0)
			out += metric("srt_conns_bytes_received_drop", "", 0)
			out += metric("srt_conns_bytes_received_undecrypt", "", 0)
			out += metricFloat("srt_conns_us_packets_send_period", "", 0)
			out += metric("srt_conns_packets_flow_window", "", 0)
			out += metric("srt_conns_packets_flight_size", "", 0)
			out += metricFloat("srt_conns_ms_rtt", "", 0)
			out += metricFloat("srt_conns_mbps_send_rate", "", 0)
			out += metricFloat("srt_conns_mbps_receive_rate", "", 0)
			out += metricFloat("srt_conns_mbps_link_capacity", "", 0)
			out += metric("srt_conns_bytes_avail_send_buf", "", 0)
			out += metric("srt_conns_bytes_avail_receive_buf", "", 0)
			out += metricFloat("srt_conns_mbps_max_bw", "", 0)
			out += metric("srt_conns_bytes_mss", "", 0)
			out += metric("srt_conns_packets_send_buf", "", 0)
			out += metric("srt_conns_bytes_send_buf", "", 0)
			out += metric("srt_conns_ms_send_buf", "", 0)
			out += metric("srt_conns_ms_send_tsb_pd_delay", "", 0)
			out += metric("srt_conns_packets_receive_buf", "", 0)
			out += metric("srt_conns_bytes_receive_buf", "", 0)
			out += metric("srt_conns_ms_receive_buf", "", 0)
			out += metric("srt_conns_ms_receive_tsb_pd_delay", "", 0)
			out += metric("srt_conns_packets_reorder_tolerance", "", 0)
			out += metric("srt_conns_packets_received_avg_belated_time", "", 0)
			out += metricFloat("srt_conns_packets_send_loss_rate", "", 0)
			out += metricFloat("srt_conns_packets_received_loss_rate", "", 0)
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
					out += metric("webrtc_sessions", ta, 1)
					out += metric("webrtc_sessions_bytes_received", ta, int64(i.BytesReceived))
					out += metric("webrtc_sessions_bytes_sent", ta, int64(i.BytesSent))
					out += metric("webrtc_sessions_rtp_packets_received", ta, int64(i.RTPPacketsReceived))
					out += metric("webrtc_sessions_rtp_packets_sent", ta, int64(i.RTPPacketsSent))
					out += metric("webrtc_sessions_rtp_packets_lost", ta, int64(i.RTPPacketsLost))
					out += metricFloat("webrtc_sessions_rtp_packets_jitter", ta, i.RTPPacketsJitter)
					out += metric("webrtc_sessions_rtcp_packets_received", ta, int64(i.RTCPPacketsReceived))
					out += metric("webrtc_sessions_rtcp_packets_sent", ta, int64(i.RTCPPacketsSent))
				}
			}
		} else if webrtcSessionFilter == "" {
			out += metric("webrtc_sessions", "", 0)
			out += metric("webrtc_sessions_bytes_received", "", 0)
			out += metric("webrtc_sessions_bytes_sent", "", 0)
			out += metric("webrtc_sessions_rtp_packets_received", "", 0)
			out += metric("webrtc_sessions_rtp_packets_sent", "", 0)
			out += metric("webrtc_sessions_rtp_packets_lost", "", 0)
			out += metricFloat("webrtc_sessions_rtp_packets_jitter", "", 0)
			out += metric("webrtc_sessions_rtcp_packets_received", "", 0)
			out += metric("webrtc_sessions_rtcp_packets_sent", "", 0)
		}
	}

	ctx.Writer.WriteHeader(http.StatusOK)
	io.WriteString(ctx.Writer, out) //nolint:errcheck
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
