package metrics //nolint:revive

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/formatlabel"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func ptrOf[T any](v T) *T {
	p := new(T)
	*p = v
	return p
}

type dummyPathManager struct{}

func (dummyPathManager) APIPathsList() (*defs.APIPathList, error) {
	return &defs.APIPathList{
		ItemCount: 1,
		PageCount: 1,
		Items: []defs.APIPath{{
			Name:     "mypath",
			ConfName: "mypathconf",
			Source: &defs.APIPathSource{
				Type: defs.APIPathSourceTypeRTSPSession,
				ID:   "123324354",
			},
			Ready:                true,
			ReadyTime:            ptrOf(time.Date(2003, 11, 4, 23, 15, 7, 0, time.UTC)),
			Tracks:               []defs.APIPathTrackCodec{formatlabel.H264, formatlabel.H265},
			InboundBytes:         123,
			OutboundBytes:        456,
			InboundFramesInError: 7,
			BytesReceived:        123,
			BytesSent:            456,
			Readers: []defs.APIPathReader{
				{
					Type: defs.APIPathReaderTypeRTSPSession,
					ID:   "345234423",
				},
				{
					Type: defs.APIPathReaderTypeRTSPSession,
					ID:   "124123142",
				},
				{
					Type: defs.APIPathReaderTypeRTMPConn,
					ID:   "723141123",
				},
			},
		}},
	}, nil
}

func (dummyPathManager) APIPathsGet(string) (*defs.APIPath, error) {
	panic("unused")
}

type dummyHLSServer struct{}

func (dummyHLSServer) APIMuxersList() (*defs.APIHLSMuxerList, error) {
	return &defs.APIHLSMuxerList{
		ItemCount: 1,
		PageCount: 1,
		Items: []defs.APIHLSMuxer{{
			Path:                    "mypath",
			Created:                 time.Date(2003, 11, 4, 23, 15, 7, 0, time.UTC),
			LastRequest:             time.Date(2003, 11, 4, 23, 15, 7, 0, time.UTC),
			OutboundBytes:           789,
			OutboundFramesDiscarded: 12,
			BytesSent:               789,
		}},
	}, nil
}

func (dummyHLSServer) APIMuxersGet(string) (*defs.APIHLSMuxer, error) {
	panic("unused")
}

func (dummyHLSServer) APISessionsList() (*defs.APIHLSSessionList, error) {
	return &defs.APIHLSSessionList{
		ItemCount: 1,
		PageCount: 1,
		Items: []defs.APIHLSSession{{
			ID:            uuid.MustParse("18294761-f9d1-4ea9-9a35-fe265b62eb41"),
			Created:       time.Date(2003, 11, 4, 23, 15, 7, 0, time.UTC),
			RemoteAddr:    "124.5.5.5:34542",
			Path:          "mypath",
			OutboundBytes: 187,
		}},
	}, nil
}

func (dummyHLSServer) APISessionsGet(uuid.UUID) (*defs.APIHLSSession, error) {
	panic("unused")
}

func (dummyHLSServer) APISessionsKick(uuid.UUID) error {
	panic("unused")
}

type dummyRTSPServer struct{}

func (dummyRTSPServer) APIConnsList() (*defs.APIRTSPConnsList, error) {
	return &defs.APIRTSPConnsList{
		ItemCount: 1,
		PageCount: 1,
		Items: []defs.APIRTSPConn{{
			ID:            uuid.MustParse("18294761-f9d1-4ea9-9a35-fe265b62eb41"),
			Created:       time.Date(2003, 11, 4, 23, 15, 7, 0, time.UTC),
			RemoteAddr:    "124.5.5.5:34542",
			InboundBytes:  123,
			OutboundBytes: 456,
			BytesReceived: 123,
			BytesSent:     456,
			Session:       nil,
		}},
	}, nil
}

func (dummyRTSPServer) APIConnsGet(uuid.UUID) (*defs.APIRTSPConn, error) {
	panic("unused")
}

func (dummyRTSPServer) APISessionsList() (*defs.APIRTSPSessionList, error) {
	return &defs.APIRTSPSessionList{
		ItemCount: 1,
		PageCount: 1,
		Items: []defs.APIRTSPSession{{
			ID:                             uuid.MustParse("124b22ce-9c34-4387-b045-44caf98049f7"),
			Created:                        time.Date(2003, 11, 4, 23, 15, 7, 0, time.UTC),
			RemoteAddr:                     "124.5.5.5:34542",
			State:                          defs.APIRTSPSessionStatePublish,
			Path:                           "mypath",
			Query:                          "myquery",
			Transport:                      nil,
			InboundBytes:                   123,
			InboundRTPPackets:              789,
			InboundRTPPacketsLost:          456,
			InboundRTPPacketsInError:       789,
			InboundRTPPacketsJitter:        123,
			InboundRTCPPackets:             456,
			InboundRTCPPacketsInError:      456,
			OutboundBytes:                  456,
			OutboundRTPPackets:             123,
			OutboundRTPPacketsReportedLost: 321,
			OutboundRTPPacketsDiscarded:    111,
			OutboundRTCPPackets:            789,
			BytesReceived:                  123,
			BytesSent:                      456,
			RTPPacketsReceived:             789,
			RTPPacketsSent:                 123,
			RTPPacketsLost:                 456,
			RTPPacketsInError:              789,
			RTPPacketsJitter:               123,
			RTCPPacketsReceived:            456,
			RTCPPacketsSent:                789,
			RTCPPacketsInError:             456,
		}},
	}, nil
}

func (dummyRTSPServer) APISessionsGet(uuid.UUID) (*defs.APIRTSPSession, error) {
	panic("unused")
}

func (dummyRTSPServer) APISessionsKick(uuid.UUID) error {
	panic("unused")
}

type dummyRTMPServer struct{}

func (dummyRTMPServer) APIConnsList() (*defs.APIRTMPConnList, error) {
	return &defs.APIRTMPConnList{
		ItemCount: 1,
		PageCount: 1,
		Items: []defs.APIRTMPConn{{
			ID:                      uuid.MustParse("9a07afe4-fc07-4c9b-be6e-6255720c36d0"),
			Created:                 time.Date(2003, 11, 4, 23, 15, 7, 0, time.UTC),
			RemoteAddr:              "3.3.3.3:5678",
			State:                   defs.APIRTMPConnStateRead,
			Path:                    "mypath",
			Query:                   "myquery",
			InboundBytes:            123,
			OutboundBytes:           456,
			OutboundFramesDiscarded: 12,
			BytesReceived:           123,
			BytesSent:               456,
		}},
	}, nil
}

func (dummyRTMPServer) APIConnsGet(uuid.UUID) (*defs.APIRTMPConn, error) {
	panic("unused")
}

func (dummyRTMPServer) APIConnsKick(uuid.UUID) error {
	panic("unused")
}

type dummySRTServer struct{}

func (dummySRTServer) APIConnsList() (*defs.APISRTConnList, error) {
	return &defs.APISRTConnList{
		ItemCount: 1,
		PageCount: 1,
		Items: []defs.APISRTConn{{
			ID:                            uuid.MustParse("a0b1c2d3-e4f5-6789-abcd-ef0123456789"),
			Created:                       time.Date(2003, 11, 4, 23, 15, 7, 0, time.UTC),
			RemoteAddr:                    "5.5.5.5:4321",
			State:                         defs.APISRTConnStatePublish,
			Path:                          "mypath",
			Query:                         "myquery",
			PacketsSent:                   100,
			PacketsReceived:               200,
			PacketsSentUnique:             90,
			PacketsReceivedUnique:         180,
			PacketsSendLoss:               10,
			PacketsReceivedLoss:           20,
			PacketsRetrans:                5,
			PacketsReceivedRetrans:        3,
			PacketsSentACK:                50,
			PacketsReceivedACK:            40,
			PacketsSentNAK:                2,
			PacketsReceivedNAK:            1,
			PacketsSentKM:                 4,
			PacketsReceivedKM:             3,
			UsSndDuration:                 1000000,
			PacketsReceivedBelated:        7,
			PacketsSendDrop:               8,
			PacketsReceivedDrop:           6,
			PacketsReceivedUndecrypt:      0,
			BytesSent:                     12345,
			BytesReceived:                 67890,
			BytesSentUnique:               11000,
			BytesReceivedUnique:           60000,
			BytesReceivedLoss:             500,
			BytesRetrans:                  300,
			BytesReceivedRetrans:          200,
			BytesReceivedBelated:          100,
			BytesSendDrop:                 400,
			BytesReceivedDrop:             350,
			BytesReceivedUndecrypt:        0,
			UsPacketsSendPeriod:           10.5,
			PacketsFlowWindow:             8192,
			PacketsFlightSize:             25,
			MsRTT:                         1.5,
			MbpsSendRate:                  50.0,
			MbpsReceiveRate:               48.0,
			MbpsLinkCapacity:              100.0,
			BytesAvailSendBuf:             65536,
			BytesAvailReceiveBuf:          65536,
			MbpsMaxBW:                     100.0,
			ByteMSS:                       1500,
			PacketsSendBuf:                10,
			BytesSendBuf:                  15000,
			MsSendBuf:                     120,
			MsSendTsbPdDelay:              120,
			PacketsReceiveBuf:             15,
			BytesReceiveBuf:               22500,
			MsReceiveBuf:                  120,
			MsReceiveTsbPdDelay:           120,
			PacketsReorderTolerance:       3,
			PacketsReceivedAvgBelatedTime: 50,
			PacketsSendLossRate:           0.01,
			PacketsReceivedLossRate:       0.02,
			OutboundFramesDiscarded:       5,
		}},
	}, nil
}

func (dummySRTServer) APIConnsGet(uuid.UUID) (*defs.APISRTConn, error) {
	panic("unused")
}

func (dummySRTServer) APIConnsKick(uuid.UUID) error {
	panic("unused")
}

type dummyWebRTCServer struct{}

func (dummyWebRTCServer) APISessionsList() (*defs.APIWebRTCSessionList, error) {
	return &defs.APIWebRTCSessionList{
		ItemCount: 1,
		PageCount: 1,
		Items: []defs.APIWebRTCSession{{
			ID:                        uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
			Created:                   time.Date(2003, 11, 4, 23, 15, 7, 0, time.UTC),
			RemoteAddr:                "127.0.0.1:3455",
			PeerConnectionEstablished: true,
			LocalCandidate:            "local",
			RemoteCandidate:           "remote",
			State:                     defs.APIWebRTCSessionStateRead,
			Path:                      "mypath",
			Query:                     "myquery",
			InboundBytes:              123,
			InboundRTPPackets:         789,
			InboundRTPPacketsLost:     456,
			InboundRTPPacketsJitter:   789,
			InboundRTCPPackets:        123,
			OutboundBytes:             456,
			OutboundRTPPackets:        123,
			OutboundRTCPPackets:       456,
			OutboundFramesDiscarded:   12,
			BytesReceived:             123,
			BytesSent:                 456,
			RTPPacketsReceived:        789,
			RTPPacketsSent:            123,
			RTPPacketsLost:            456,
			RTPPacketsJitter:          789,
			RTCPPacketsReceived:       123,
			RTCPPacketsSent:           456,
		}},
	}, nil
}

func (dummyWebRTCServer) APISessionsGet(uuid.UUID) (*defs.APIWebRTCSession, error) {
	panic("unused")
}

func (dummyWebRTCServer) APISessionsKick(uuid.UUID) error {
	panic("unused")
}

type emptyPathManager struct{}

func (emptyPathManager) APIPathsList() (*defs.APIPathList, error) {
	return &defs.APIPathList{}, nil
}

func (emptyPathManager) APIPathsGet(string) (*defs.APIPath, error) {
	panic("unused")
}

type emptyHLSServer struct{}

func (emptyHLSServer) APIMuxersList() (*defs.APIHLSMuxerList, error) {
	return &defs.APIHLSMuxerList{}, nil
}

func (emptyHLSServer) APIMuxersGet(string) (*defs.APIHLSMuxer, error) {
	panic("unused")
}

func (emptyHLSServer) APISessionsList() (*defs.APIHLSSessionList, error) {
	return &defs.APIHLSSessionList{}, nil
}

func (emptyHLSServer) APISessionsGet(uuid.UUID) (*defs.APIHLSSession, error) {
	panic("unused")
}

func (emptyHLSServer) APISessionsKick(uuid.UUID) error {
	panic("unused")
}

type emptyRTSPServer struct{}

func (emptyRTSPServer) APIConnsList() (*defs.APIRTSPConnsList, error) {
	return &defs.APIRTSPConnsList{}, nil
}

func (emptyRTSPServer) APIConnsGet(uuid.UUID) (*defs.APIRTSPConn, error) {
	panic("unused")
}

func (emptyRTSPServer) APISessionsList() (*defs.APIRTSPSessionList, error) {
	return &defs.APIRTSPSessionList{}, nil
}

func (emptyRTSPServer) APISessionsGet(uuid.UUID) (*defs.APIRTSPSession, error) {
	panic("unused")
}

func (emptyRTSPServer) APISessionsKick(uuid.UUID) error {
	panic("unused")
}

type emptySRTServer struct{}

func (emptySRTServer) APIConnsList() (*defs.APISRTConnList, error) {
	return &defs.APISRTConnList{}, nil
}

func (emptySRTServer) APIConnsGet(uuid.UUID) (*defs.APISRTConn, error) {
	panic("unused")
}

func (emptySRTServer) APIConnsKick(uuid.UUID) error {
	panic("unused")
}

type emptyWebRTCServer struct{}

func (emptyWebRTCServer) APISessionsList() (*defs.APIWebRTCSessionList, error) {
	return &defs.APIWebRTCSessionList{}, nil
}

func (emptyWebRTCServer) APISessionsGet(uuid.UUID) (*defs.APIWebRTCSession, error) {
	panic("unused")
}

func (emptyWebRTCServer) APISessionsKick(uuid.UUID) error {
	panic("unused")
}

func TestPreflightRequest(t *testing.T) {
	m := Metrics{
		Address:      "localhost:9998",
		AllowOrigins: []string{"*"},
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		Parent:       test.NilLogger,
	}
	err := m.Initialize()
	require.NoError(t, err)
	defer m.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	req, err := http.NewRequest(http.MethodOptions, "http://localhost:9998", nil)
	require.NoError(t, err)

	req.Header.Add("Access-Control-Request-Method", "GET")

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNoContent, res.StatusCode)

	byts, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	require.Equal(t, "*", res.Header.Get("Access-Control-Allow-Origin"))
	require.Equal(t, "true", res.Header.Get("Access-Control-Allow-Credentials"))
	require.Equal(t, "OPTIONS, GET", res.Header.Get("Access-Control-Allow-Methods"))
	require.Equal(t, "Authorization", res.Header.Get("Access-Control-Allow-Headers"))
	require.Equal(t, byts, []byte{})
}

func TestMetrics(t *testing.T) {
	checked := false

	m := Metrics{
		Address:      "localhost:9998",
		AllowOrigins: []string{"*"},
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager: &test.AuthManager{
			AuthenticateImpl: func(req *auth.Request) (string, *auth.Error) {
				require.Equal(t, conf.AuthActionMetrics, req.Action)
				require.Equal(t, "myuser", req.Credentials.User)
				require.Equal(t, "mypass", req.Credentials.Pass)
				checked = true
				return req.Credentials.User, nil
			},
		},
		Parent: test.NilLogger,
	}
	err := m.Initialize()
	require.NoError(t, err)
	defer m.Close()

	m.SetPathManager(&dummyPathManager{})
	m.SetHLSServer(&dummyHLSServer{})
	m.SetRTSPServer(&dummyRTSPServer{})
	m.SetRTSPSServer(&dummyRTSPServer{})
	m.SetRTMPServer(&dummyRTMPServer{})
	m.SetRTMPSServer(&dummyRTMPServer{})
	m.SetSRTServer(&dummySRTServer{})
	m.SetWebRTCServer(&dummyWebRTCServer{})

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	res, err := hc.Get("http://myuser:mypass@localhost:9998/metrics")
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)

	byts, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	require.Equal(t,
		"# Paths\n"+ //nolint:dupl
			"paths{name=\"mypath\",state=\"ready\"} 1\n"+
			"paths_readers{name=\"mypath\",readerType=\"rtmpConn\",state=\"ready\"} 1\n"+
			"paths_readers{name=\"mypath\",readerType=\"rtspSession\",state=\"ready\"} 2\n"+
			"paths_inbound_bytes{name=\"mypath\",state=\"ready\"} 123\n"+
			"paths_outbound_bytes{name=\"mypath\",state=\"ready\"} 456\n"+
			"paths_inbound_frames_in_error{name=\"mypath\",state=\"ready\"} 7\n"+
			"\n"+
			"# Paths (deprecated)\n"+
			"paths_bytes_received{name=\"mypath\",state=\"ready\"} 123\n"+
			"paths_bytes_sent{name=\"mypath\",state=\"ready\"} 456\n"+
			"\n"+
			"# HLS sessions\n"+
			"hls_sessions{id=\"18294761-f9d1-4ea9-9a35-fe265b62eb41\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\"} 1\n"+
			"hls_sessions_outbound_bytes{id=\"18294761-f9d1-4ea9-9a35-fe265b62eb41\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\"} 187\n"+
			"\n"+
			"# HLS muxers\n"+
			"hls_muxers{name=\"mypath\"} 1\n"+
			"hls_muxers_outbound_bytes{name=\"mypath\"} 789\n"+
			"hls_muxers_outbound_frames_discarded{name=\"mypath\"} 12\n"+
			"\n"+
			"# HLS muxers (deprecated)\n"+
			"hls_muxers_bytes_sent{name=\"mypath\"} 789\n"+
			"\n"+
			"# RTSP connections\n"+
			"rtsp_conns{id=\"18294761-f9d1-4ea9-9a35-fe265b62eb41\"} 1\n"+
			"rtsp_conns_inbound_bytes{id=\"18294761-f9d1-4ea9-9a35-fe265b62eb41\"} 123\n"+
			"rtsp_conns_outbound_bytes{id=\"18294761-f9d1-4ea9-9a35-fe265b62eb41\"} 456\n"+
			"\n"+
			"# RTSP connections (deprecated)\n"+
			"rtsp_conns_bytes_received{id=\"18294761-f9d1-4ea9-9a35-fe265b62eb41\"} 123\n"+
			"rtsp_conns_bytes_sent{id=\"18294761-f9d1-4ea9-9a35-fe265b62eb41\"} 456\n"+
			"\n"+
			"# RTSP sessions\n"+
			"rtsp_sessions{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 1\n"+
			"rtsp_sessions_inbound_bytes{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 123\n"+
			"rtsp_sessions_inbound_rtp_packets{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 789\n"+
			"rtsp_sessions_inbound_rtp_packets_lost{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 456\n"+
			"rtsp_sessions_inbound_rtp_packets_in_error{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 789\n"+
			"rtsp_sessions_inbound_rtp_packets_jitter{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 123\n"+
			"rtsp_sessions_inbound_rtcp_packets{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 456\n"+
			"rtsp_sessions_inbound_rtcp_packets_in_error{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 456\n"+
			"rtsp_sessions_outbound_bytes{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 456\n"+
			"rtsp_sessions_outbound_rtp_packets{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 123\n"+
			"rtsp_sessions_outbound_rtp_packets_reported_lost{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 321\n"+
			"rtsp_sessions_outbound_rtp_packets_discarded{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 111\n"+
			"rtsp_sessions_outbound_rtcp_packets{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 789\n"+
			"\n"+
			"# RTSP sessions (deprecated)\n"+
			"rtsp_sessions_bytes_received{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 123\n"+
			"rtsp_sessions_bytes_sent{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 456\n"+
			"rtsp_sessions_rtp_packets_received{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 789\n"+
			"rtsp_sessions_rtp_packets_sent{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 123\n"+
			"rtsp_sessions_rtp_packets_lost{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 456\n"+
			"rtsp_sessions_rtp_packets_in_error{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 789\n"+
			"rtsp_sessions_rtp_packets_jitter{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 123\n"+
			"rtsp_sessions_rtcp_packets_received{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 456\n"+
			"rtsp_sessions_rtcp_packets_sent{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 789\n"+
			"rtsp_sessions_rtcp_packets_in_error{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 456\n"+
			"\n"+
			"# RTSPS connections\n"+
			"rtsps_conns{id=\"18294761-f9d1-4ea9-9a35-fe265b62eb41\"} 1\n"+
			"rtsps_conns_inbound_bytes{id=\"18294761-f9d1-4ea9-9a35-fe265b62eb41\"} 123\n"+
			"rtsps_conns_outbound_bytes{id=\"18294761-f9d1-4ea9-9a35-fe265b62eb41\"} 456\n"+
			"\n"+
			"# RTSPS connections (deprecated)\n"+
			"rtsps_conns_bytes_received{id=\"18294761-f9d1-4ea9-9a35-fe265b62eb41\"} 123\n"+
			"rtsps_conns_bytes_sent{id=\"18294761-f9d1-4ea9-9a35-fe265b62eb41\"} 456\n"+
			"\n"+
			"# RTSPS sessions\n"+
			"rtsps_sessions{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 1\n"+
			"rtsps_sessions_inbound_bytes{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 123\n"+
			"rtsps_sessions_inbound_rtp_packets{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 789\n"+
			"rtsps_sessions_inbound_rtp_packets_lost{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 456\n"+
			"rtsps_sessions_inbound_rtp_packets_in_error{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 789\n"+
			"rtsps_sessions_inbound_rtp_packets_jitter{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 123\n"+
			"rtsps_sessions_inbound_rtcp_packets{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 456\n"+
			"rtsps_sessions_inbound_rtcp_packets_in_error{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 456\n"+
			"rtsps_sessions_outbound_bytes{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 456\n"+
			"rtsps_sessions_outbound_rtp_packets{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 123\n"+
			"rtsps_sessions_outbound_rtp_packets_reported_lost{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 321\n"+
			"rtsps_sessions_outbound_rtp_packets_discarded{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 111\n"+
			"rtsps_sessions_outbound_rtcp_packets{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 789\n"+
			"\n"+
			"# RTSPS sessions (deprecated)\n"+
			"rtsps_sessions_bytes_received{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 123\n"+
			"rtsps_sessions_bytes_sent{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 456\n"+
			"rtsps_sessions_rtp_packets_received{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 789\n"+
			"rtsps_sessions_rtp_packets_sent{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 123\n"+
			"rtsps_sessions_rtp_packets_lost{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 456\n"+
			"rtsps_sessions_rtp_packets_in_error{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 789\n"+
			"rtsps_sessions_rtp_packets_jitter{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 123\n"+
			"rtsps_sessions_rtcp_packets_received{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 456\n"+
			"rtsps_sessions_rtcp_packets_sent{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 789\n"+
			"rtsps_sessions_rtcp_packets_in_error{id=\"124b22ce-9c34-4387-b045-44caf98049f7\",path=\"mypath\","+
			"remoteAddr=\"124.5.5.5:34542\",state=\"publish\"} 456\n"+
			"\n"+
			"# RTMP connections\n"+
			"rtmp_conns{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
			"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 1\n"+
			"rtmp_conns_inbound_bytes{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
			"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 123\n"+
			"rtmp_conns_outbound_bytes{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
			"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 456\n"+
			"rtmp_conns_outbound_frames_discarded{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
			"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 12\n"+
			"\n"+
			"# RTMP connections (deprecated)\n"+
			"rtmp_conns_bytes_received{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
			"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 123\n"+
			"rtmp_conns_bytes_sent{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
			"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 456\n"+
			"\n"+
			"# RTMPS connections\n"+
			"rtmps_conns{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
			"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 1\n"+
			"rtmps_conns_inbound_bytes{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
			"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 123\n"+
			"rtmps_conns_outbound_bytes{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
			"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 456\n"+
			"rtmps_conns_outbound_frames_discarded{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
			"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 12\n"+
			"\n"+
			"# RTMPS connections (deprecated)\n"+
			"rtmps_conns_bytes_received{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
			"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 123\n"+
			"rtmps_conns_bytes_sent{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
			"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 456\n"+
			"\n"+
			"# SRT connections\n"+
			"srt_conns{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 1\n"+
			"srt_conns_packets_sent{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 100\n"+
			"srt_conns_packets_received{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 200\n"+
			"srt_conns_packets_sent_unique{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 90\n"+
			"srt_conns_packets_received_unique{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 180\n"+
			"srt_conns_packets_send_loss{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 10\n"+
			"srt_conns_packets_received_loss{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 20\n"+
			"srt_conns_packets_retrans{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 5\n"+
			"srt_conns_packets_received_retrans{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 3\n"+
			"srt_conns_packets_sent_ack{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 50\n"+
			"srt_conns_packets_received_ack{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 40\n"+
			"srt_conns_packets_sent_nak{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 2\n"+
			"srt_conns_packets_received_nak{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 1\n"+
			"srt_conns_packets_sent_km{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 4\n"+
			"srt_conns_packets_received_km{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 3\n"+
			"srt_conns_us_snd_duration{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 1000000\n"+
			"srt_conns_packets_received_belated{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 7\n"+
			"srt_conns_packets_send_drop{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 8\n"+
			"srt_conns_packets_received_drop{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 6\n"+
			"srt_conns_packets_received_undecrypt{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 0\n"+
			"srt_conns_bytes_sent{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 12345\n"+
			"srt_conns_bytes_received{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 67890\n"+
			"srt_conns_bytes_sent_unique{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 11000\n"+
			"srt_conns_bytes_received_unique{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 60000\n"+
			"srt_conns_bytes_received_loss{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 500\n"+
			"srt_conns_bytes_retrans{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 300\n"+
			"srt_conns_bytes_received_retrans{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 200\n"+
			"srt_conns_bytes_received_belated{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 100\n"+
			"srt_conns_bytes_send_drop{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 400\n"+
			"srt_conns_bytes_received_drop{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 350\n"+
			"srt_conns_bytes_received_undecrypt{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 0\n"+
			"srt_conns_us_packets_send_period{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 10.5\n"+
			"srt_conns_packets_flow_window{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 8192\n"+
			"srt_conns_packets_flight_size{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 25\n"+
			"srt_conns_ms_rtt{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 1.5\n"+
			"srt_conns_mbps_send_rate{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 50\n"+
			"srt_conns_mbps_receive_rate{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 48\n"+
			"srt_conns_mbps_link_capacity{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 100\n"+
			"srt_conns_bytes_avail_send_buf{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 65536\n"+
			"srt_conns_bytes_avail_receive_buf{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 65536\n"+
			"srt_conns_mbps_max_bw{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 100\n"+
			"srt_conns_bytes_mss{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 1500\n"+
			"srt_conns_packets_send_buf{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 10\n"+
			"srt_conns_bytes_send_buf{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 15000\n"+
			"srt_conns_ms_send_buf{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 120\n"+
			"srt_conns_ms_send_tsb_pd_delay{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 120\n"+
			"srt_conns_packets_receive_buf{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 15\n"+
			"srt_conns_bytes_receive_buf{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 22500\n"+
			"srt_conns_ms_receive_buf{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 120\n"+
			"srt_conns_ms_receive_tsb_pd_delay{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 120\n"+
			"srt_conns_packets_reorder_tolerance{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 3\n"+
			"srt_conns_packets_received_avg_belated_time{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 50\n"+
			"srt_conns_packets_send_loss_rate{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 0.01\n"+
			"srt_conns_packets_received_loss_rate{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 0.02\n"+
			"srt_conns_outbound_frames_discarded{id=\"a0b1c2d3-e4f5-6789-abcd-ef0123456789\",path=\"mypath\","+
			"remoteAddr=\"5.5.5.5:4321\",state=\"publish\"} 5\n"+
			"\n"+
			"# WebRTC sessions\n"+
			"webrtc_sessions{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
			"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 1\n"+
			"webrtc_sessions_inbound_bytes{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
			"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 123\n"+
			"webrtc_sessions_inbound_rtp_packets{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
			"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 789\n"+
			"webrtc_sessions_inbound_rtp_packets_lost{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
			"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 456\n"+
			"webrtc_sessions_inbound_rtp_packets_jitter{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
			"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 789\n"+
			"webrtc_sessions_inbound_rtcp_packets{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
			"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 123\n"+
			"webrtc_sessions_outbound_bytes{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
			"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 456\n"+
			"webrtc_sessions_outbound_rtp_packets{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
			"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 123\n"+
			"webrtc_sessions_outbound_rtcp_packets{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
			"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 456\n"+
			"webrtc_sessions_outbound_frames_discarded{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
			"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 12\n"+
			"\n"+
			"# WebRTC sessions (deprecated)\n"+
			"webrtc_sessions_bytes_received{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
			"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 123\n"+
			"webrtc_sessions_bytes_sent{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
			"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 456\n"+
			"webrtc_sessions_rtp_packets_received{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
			"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 789\n"+
			"webrtc_sessions_rtp_packets_sent{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
			"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 123\n"+
			"webrtc_sessions_rtp_packets_lost{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
			"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 456\n"+
			"webrtc_sessions_rtp_packets_jitter{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
			"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 789\n"+
			"webrtc_sessions_rtcp_packets_received{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
			"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 123\n"+
			"webrtc_sessions_rtcp_packets_sent{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
			"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 456\n"+
			"\n",
		string(byts))

	require.True(t, checked)
}

func TestZeroMetricsFallback(t *testing.T) {
	m := Metrics{
		Address:      "localhost:9998",
		AllowOrigins: []string{"*"},
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		Parent:       test.NilLogger,
	}
	err := m.Initialize()
	require.NoError(t, err)
	defer m.Close()

	m.SetPathManager(&emptyPathManager{})
	m.SetHLSServer(&emptyHLSServer{})
	m.SetRTSPServer(&emptyRTSPServer{})
	m.SetSRTServer(&emptySRTServer{})
	m.SetWebRTCServer(&emptyWebRTCServer{})

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	res, err := hc.Get("http://localhost:9998/metrics")
	require.NoError(t, err)
	defer res.Body.Close()

	byts, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	require.Equal(t,
		"# Paths\n"+ //nolint:dupl
			"paths 0\n"+
			"paths_inbound_bytes 0\n"+
			"paths_outbound_bytes 0\n"+
			"paths_inbound_frames_in_error 0\n"+
			"\n"+
			"# Paths (deprecated)\n"+
			"paths_bytes_received 0\n"+
			"paths_bytes_sent 0\n"+
			"paths_readers 0\n"+
			"\n"+
			"# HLS sessions\n"+
			"hls_sessions 0\n"+
			`hls_sessions_outbound_bytes 0`+"\n"+
			"\n"+
			"# HLS muxers\n"+
			"hls_muxers 0\n"+
			"hls_muxers_outbound_bytes 0\n"+
			"hls_muxers_outbound_frames_discarded 0\n"+
			"\n"+
			"# HLS muxers (deprecated)\n"+
			"hls_muxers_bytes_sent 0\n"+
			"\n"+
			"# RTSP connections\n"+
			"rtsp_conns 0\n"+
			"rtsp_conns_inbound_bytes 0\n"+
			"rtsp_conns_outbound_bytes 0\n"+
			"\n"+
			"# RTSP connections (deprecated)\n"+
			"rtsp_conns_bytes_received 0\n"+
			"rtsp_conns_bytes_sent 0\n"+
			"\n"+
			"# RTSP sessions\n"+
			"rtsp_sessions 0\n"+
			"rtsp_sessions_inbound_bytes 0\n"+
			"rtsp_sessions_inbound_rtp_packets 0\n"+
			"rtsp_sessions_inbound_rtp_packets_lost 0\n"+
			"rtsp_sessions_inbound_rtp_packets_in_error 0\n"+
			"rtsp_sessions_inbound_rtp_packets_jitter 0\n"+
			"rtsp_sessions_inbound_rtcp_packets 0\n"+
			"rtsp_sessions_inbound_rtcp_packets_in_error 0\n"+
			"rtsp_sessions_outbound_bytes 0\n"+
			"rtsp_sessions_outbound_rtp_packets 0\n"+
			"rtsp_sessions_outbound_rtp_packets_reported_lost 0\n"+
			"rtsp_sessions_outbound_rtp_packets_discarded 0\n"+
			"rtsp_sessions_outbound_rtcp_packets 0\n"+
			"\n"+
			"# RTSP sessions (deprecated)\n"+
			"rtsp_sessions_bytes_received 0\n"+
			"rtsp_sessions_bytes_sent 0\n"+
			"rtsp_sessions_rtp_packets_received 0\n"+
			"rtsp_sessions_rtp_packets_sent 0\n"+
			"rtsp_sessions_rtp_packets_lost 0\n"+
			"rtsp_sessions_rtp_packets_in_error 0\n"+
			"rtsp_sessions_rtp_packets_jitter 0\n"+
			"rtsp_sessions_rtcp_packets_received 0\n"+
			"rtsp_sessions_rtcp_packets_sent 0\n"+
			"rtsp_sessions_rtcp_packets_in_error 0\n"+
			"\n"+
			"# SRT connections\n"+
			"srt_conns 0\n"+
			"srt_conns_packets_sent 0\n"+
			"srt_conns_packets_received 0\n"+
			"srt_conns_packets_sent_unique 0\n"+
			"srt_conns_packets_received_unique 0\n"+
			"srt_conns_packets_send_loss 0\n"+
			"srt_conns_packets_received_loss 0\n"+
			"srt_conns_packets_retrans 0\n"+
			"srt_conns_packets_received_retrans 0\n"+
			"srt_conns_packets_sent_ack 0\n"+
			"srt_conns_packets_received_ack 0\n"+
			"srt_conns_packets_sent_nak 0\n"+
			"srt_conns_packets_received_nak 0\n"+
			"srt_conns_packets_sent_km 0\n"+
			"srt_conns_packets_received_km 0\n"+
			"srt_conns_us_snd_duration 0\n"+
			"srt_conns_packets_received_belated 0\n"+
			"srt_conns_packets_send_drop 0\n"+
			"srt_conns_packets_received_drop 0\n"+
			"srt_conns_packets_received_undecrypt 0\n"+
			"srt_conns_bytes_sent 0\n"+
			"srt_conns_bytes_received 0\n"+
			"srt_conns_bytes_sent_unique 0\n"+
			"srt_conns_bytes_received_unique 0\n"+
			"srt_conns_bytes_received_loss 0\n"+
			"srt_conns_bytes_retrans 0\n"+
			"srt_conns_bytes_received_retrans 0\n"+
			"srt_conns_bytes_received_belated 0\n"+
			"srt_conns_bytes_send_drop 0\n"+
			"srt_conns_bytes_received_drop 0\n"+
			"srt_conns_bytes_received_undecrypt 0\n"+
			"srt_conns_us_packets_send_period 0\n"+
			"srt_conns_packets_flow_window 0\n"+
			"srt_conns_packets_flight_size 0\n"+
			"srt_conns_ms_rtt 0\n"+
			"srt_conns_mbps_send_rate 0\n"+
			"srt_conns_mbps_receive_rate 0\n"+
			"srt_conns_mbps_link_capacity 0\n"+
			"srt_conns_bytes_avail_send_buf 0\n"+
			"srt_conns_bytes_avail_receive_buf 0\n"+
			"srt_conns_mbps_max_bw 0\n"+
			"srt_conns_bytes_mss 0\n"+
			"srt_conns_packets_send_buf 0\n"+
			"srt_conns_bytes_send_buf 0\n"+
			"srt_conns_ms_send_buf 0\n"+
			"srt_conns_ms_send_tsb_pd_delay 0\n"+
			"srt_conns_packets_receive_buf 0\n"+
			"srt_conns_bytes_receive_buf 0\n"+
			"srt_conns_ms_receive_buf 0\n"+
			"srt_conns_ms_receive_tsb_pd_delay 0\n"+
			"srt_conns_packets_reorder_tolerance 0\n"+
			"srt_conns_packets_received_avg_belated_time 0\n"+
			"srt_conns_packets_send_loss_rate 0\n"+
			"srt_conns_packets_received_loss_rate 0\n"+
			"srt_conns_outbound_frames_discarded 0\n"+
			"\n"+
			"# WebRTC sessions\n"+
			"webrtc_sessions 0\n"+
			"webrtc_sessions_inbound_bytes 0\n"+
			"webrtc_sessions_inbound_rtp_packets 0\n"+
			"webrtc_sessions_inbound_rtp_packets_lost 0\n"+
			"webrtc_sessions_inbound_rtp_packets_jitter 0\n"+
			"webrtc_sessions_inbound_rtcp_packets 0\n"+
			"webrtc_sessions_outbound_bytes 0\n"+
			"webrtc_sessions_outbound_rtp_packets 0\n"+
			"webrtc_sessions_outbound_rtcp_packets 0\n"+
			"webrtc_sessions_outbound_frames_discarded 0\n"+
			"\n"+
			"# WebRTC sessions (deprecated)\n"+
			"webrtc_sessions_bytes_received 0\n"+
			"webrtc_sessions_bytes_sent 0\n"+
			"webrtc_sessions_rtp_packets_received 0\n"+
			"webrtc_sessions_rtp_packets_sent 0\n"+
			"webrtc_sessions_rtp_packets_lost 0\n"+
			"webrtc_sessions_rtp_packets_jitter 0\n"+
			"webrtc_sessions_rtcp_packets_received 0\n"+
			"webrtc_sessions_rtcp_packets_sent 0\n"+
			"\n",
		string(byts))
}

func TestFilter(t *testing.T) {
	for _, ca := range []string{
		"path",
		"hls_muxer",
		"hls_session",
		"rtsp_conn",
		"rtsp_session",
		"rtsps_conn",
		"rtsps_session",
		"rtmp_conn",
		"rtmps_conn",
		"srt_conn",
		"webrtc_session",
	} {
		t.Run(ca, func(t *testing.T) {
			m := Metrics{
				Address:      "localhost:9998",
				AllowOrigins: []string{"*"},
				ReadTimeout:  conf.Duration(10 * time.Second),
				WriteTimeout: conf.Duration(10 * time.Second),
				AuthManager:  test.NilAuthManager,
				Parent:       test.NilLogger,
			}
			err := m.Initialize()
			require.NoError(t, err)
			defer m.Close()

			m.SetPathManager(&dummyPathManager{})
			m.SetHLSServer(&dummyHLSServer{})
			m.SetRTSPServer(&dummyRTSPServer{})
			m.SetRTSPSServer(&dummyRTSPServer{})
			m.SetRTMPServer(&dummyRTMPServer{})
			m.SetRTMPSServer(&dummyRTMPServer{})
			m.SetSRTServer(&dummySRTServer{})
			m.SetWebRTCServer(&dummyWebRTCServer{})

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr}

			u := "http://localhost:9998/metrics"

			switch ca {
			case "path":
				u += "?path=mypath"
			case "hls_muxer":
				u += "?hls_muxer=mypath"
			case "hls_session":
				u += "?hls_session=18294761-f9d1-4ea9-9a35-fe265b62eb41"
			case "rtsp_conn":
				u += "?rtsp_conn=18294761-f9d1-4ea9-9a35-fe265b62eb41"
			case "rtsp_session":
				u += "?rtsp_session=124b22ce-9c34-4387-b045-44caf98049f7"
			case "rtsps_conn":
				u += "?rtsps_conn=18294761-f9d1-4ea9-9a35-fe265b62eb41"
			case "rtsps_session":
				u += "?rtsps_session=124b22ce-9c34-4387-b045-44caf98049f7"
			case "rtmp_conn":
				u += "?rtmp_conn=9a07afe4-fc07-4c9b-be6e-6255720c36d0"
			case "rtmps_conn":
				u += "?rtmps_conn=9a07afe4-fc07-4c9b-be6e-6255720c36d0"
			case "srt_conn":
				u += "?srt_conn=a0b1c2d3-e4f5-6789-abcd-ef0123456789"
			case "webrtc_session":
				u += "?webrtc_session=f47ac10b-58cc-4372-a567-0e02b2c3d479"
			}

			res, err := hc.Get(u)
			require.NoError(t, err)
			defer res.Body.Close()

			byts, err := io.ReadAll(res.Body)
			require.NoError(t, err)

			switch ca {
			case "path":
				require.Equal(t,
					"# Paths\n"+
						`paths{name="mypath",state="ready"} 1`+"\n"+
						`paths_readers{name="mypath",readerType="rtmpConn",state="ready"} 1`+"\n"+
						`paths_readers{name="mypath",readerType="rtspSession",state="ready"} 2`+"\n"+
						`paths_inbound_bytes{name="mypath",state="ready"} 123`+"\n"+
						`paths_outbound_bytes{name="mypath",state="ready"} 456`+"\n"+
						`paths_inbound_frames_in_error{name="mypath",state="ready"} 7`+"\n"+
						"\n"+
						"# Paths (deprecated)\n"+
						`paths_bytes_received{name="mypath",state="ready"} 123`+"\n"+
						`paths_bytes_sent{name="mypath",state="ready"} 456`+"\n\n",
					string(byts))

			case "hls_muxer":
				require.Equal(t,
					"# HLS muxers\n"+
						`hls_muxers{name="mypath"} 1`+"\n"+
						`hls_muxers_outbound_bytes{name="mypath"} 789`+"\n"+
						`hls_muxers_outbound_frames_discarded{name="mypath"} 12`+"\n"+
						"\n"+
						"# HLS muxers (deprecated)\n"+
						`hls_muxers_bytes_sent{name="mypath"} 789`+"\n\n",
					string(byts))

			case "hls_session":
				require.Equal(t,
					"# HLS sessions\n"+
						`hls_sessions{id="18294761-f9d1-4ea9-9a35-fe265b62eb41",path="mypath",`+
						`remoteAddr="124.5.5.5:34542"} 1`+"\n"+
						`hls_sessions_outbound_bytes{id="18294761-f9d1-4ea9-9a35-fe265b62eb41",path="mypath",`+
						`remoteAddr="124.5.5.5:34542"} 187`+"\n"+
						"\n",
					string(byts))

			case "rtsp_conn":
				require.Equal(t,
					"# RTSP connections\n"+
						`rtsp_conns{id="18294761-f9d1-4ea9-9a35-fe265b62eb41"} 1`+"\n"+
						`rtsp_conns_inbound_bytes{id="18294761-f9d1-4ea9-9a35-fe265b62eb41"} 123`+"\n"+
						`rtsp_conns_outbound_bytes{id="18294761-f9d1-4ea9-9a35-fe265b62eb41"} 456`+"\n"+
						"\n"+
						"# RTSP connections (deprecated)\n"+
						`rtsp_conns_bytes_received{id="18294761-f9d1-4ea9-9a35-fe265b62eb41"} 123`+"\n"+
						`rtsp_conns_bytes_sent{id="18294761-f9d1-4ea9-9a35-fe265b62eb41"} 456`+"\n\n",
					string(byts))

			case "rtsp_session": //nolint:dupl
				require.Equal(t,
					"# RTSP sessions\n"+
						`rtsp_sessions{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 1`+"\n"+
						`rtsp_sessions_inbound_bytes{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 123`+"\n"+
						`rtsp_sessions_inbound_rtp_packets{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 789`+"\n"+
						`rtsp_sessions_inbound_rtp_packets_lost{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 456`+"\n"+
						`rtsp_sessions_inbound_rtp_packets_in_error{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 789`+"\n"+
						`rtsp_sessions_inbound_rtp_packets_jitter{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 123`+"\n"+
						`rtsp_sessions_inbound_rtcp_packets{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 456`+"\n"+
						`rtsp_sessions_inbound_rtcp_packets_in_error{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 456`+"\n"+
						`rtsp_sessions_outbound_bytes{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 456`+"\n"+
						`rtsp_sessions_outbound_rtp_packets{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 123`+"\n"+
						`rtsp_sessions_outbound_rtp_packets_reported_lost{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 321`+"\n"+
						`rtsp_sessions_outbound_rtp_packets_discarded{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 111`+"\n"+
						`rtsp_sessions_outbound_rtcp_packets{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 789`+"\n"+
						"\n"+
						"# RTSP sessions (deprecated)\n"+
						`rtsp_sessions_bytes_received{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 123`+"\n"+
						`rtsp_sessions_bytes_sent{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 456`+"\n"+
						`rtsp_sessions_rtp_packets_received{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 789`+"\n"+
						`rtsp_sessions_rtp_packets_sent{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 123`+"\n"+
						`rtsp_sessions_rtp_packets_lost{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 456`+"\n"+
						`rtsp_sessions_rtp_packets_in_error{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 789`+"\n"+
						`rtsp_sessions_rtp_packets_jitter{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 123`+"\n"+
						`rtsp_sessions_rtcp_packets_received{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 456`+"\n"+
						`rtsp_sessions_rtcp_packets_sent{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 789`+"\n"+
						`rtsp_sessions_rtcp_packets_in_error{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 456`+"\n\n",
					string(byts))

			case "rtsps_conn":
				require.Equal(t,
					"# RTSPS connections\n"+
						`rtsps_conns{id="18294761-f9d1-4ea9-9a35-fe265b62eb41"} 1`+"\n"+
						`rtsps_conns_inbound_bytes{id="18294761-f9d1-4ea9-9a35-fe265b62eb41"} 123`+"\n"+
						`rtsps_conns_outbound_bytes{id="18294761-f9d1-4ea9-9a35-fe265b62eb41"} 456`+"\n"+
						"\n"+
						"# RTSPS connections (deprecated)\n"+
						`rtsps_conns_bytes_received{id="18294761-f9d1-4ea9-9a35-fe265b62eb41"} 123`+"\n"+
						`rtsps_conns_bytes_sent{id="18294761-f9d1-4ea9-9a35-fe265b62eb41"} 456`+"\n\n",
					string(byts))

			case "rtsps_session": //nolint:dupl
				require.Equal(t,
					"# RTSPS sessions\n"+
						`rtsps_sessions{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 1`+"\n"+
						`rtsps_sessions_inbound_bytes{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 123`+"\n"+
						`rtsps_sessions_inbound_rtp_packets{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 789`+"\n"+
						`rtsps_sessions_inbound_rtp_packets_lost{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 456`+"\n"+
						`rtsps_sessions_inbound_rtp_packets_in_error{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 789`+"\n"+
						`rtsps_sessions_inbound_rtp_packets_jitter{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 123`+"\n"+
						`rtsps_sessions_inbound_rtcp_packets{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 456`+"\n"+
						`rtsps_sessions_inbound_rtcp_packets_in_error{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 456`+"\n"+
						`rtsps_sessions_outbound_bytes{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 456`+"\n"+
						`rtsps_sessions_outbound_rtp_packets{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 123`+"\n"+
						`rtsps_sessions_outbound_rtp_packets_reported_lost{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 321`+"\n"+
						`rtsps_sessions_outbound_rtp_packets_discarded{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 111`+"\n"+
						`rtsps_sessions_outbound_rtcp_packets{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 789`+"\n"+
						"\n"+
						"# RTSPS sessions (deprecated)\n"+
						`rtsps_sessions_bytes_received{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 123`+"\n"+
						`rtsps_sessions_bytes_sent{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 456`+"\n"+
						`rtsps_sessions_rtp_packets_received{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 789`+"\n"+
						`rtsps_sessions_rtp_packets_sent{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 123`+"\n"+
						`rtsps_sessions_rtp_packets_lost{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 456`+"\n"+
						`rtsps_sessions_rtp_packets_in_error{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 789`+"\n"+
						`rtsps_sessions_rtp_packets_jitter{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 123`+"\n"+
						`rtsps_sessions_rtcp_packets_received{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 456`+"\n"+
						`rtsps_sessions_rtcp_packets_sent{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 789`+"\n"+
						`rtsps_sessions_rtcp_packets_in_error{id="124b22ce-9c34-4387-b045-44caf98049f7",`+
						`path="mypath",remoteAddr="124.5.5.5:34542",state="publish"} 456`+"\n\n",
					string(byts))

			case "rtmp_conn":
				require.Equal(t,
					"# RTMP connections\n"+
						"rtmp_conns{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
						"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 1\n"+
						"rtmp_conns_inbound_bytes{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
						"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 123\n"+
						"rtmp_conns_outbound_bytes{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
						"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 456\n"+
						"rtmp_conns_outbound_frames_discarded{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
						"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 12\n"+
						"\n"+
						"# RTMP connections (deprecated)\n"+
						"rtmp_conns_bytes_received{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
						"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 123\n"+
						"rtmp_conns_bytes_sent{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
						"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 456\n\n",
					string(byts))

			case "rtmps_conn":
				require.Equal(t,
					"# RTMPS connections\n"+
						`rtmps_conns{id="9a07afe4-fc07-4c9b-be6e-6255720c36d0",`+
						`path="mypath",remoteAddr="3.3.3.3:5678",state="read"} 1`+"\n"+
						`rtmps_conns_inbound_bytes{id="9a07afe4-fc07-4c9b-be6e-6255720c36d0",`+
						`path="mypath",remoteAddr="3.3.3.3:5678",state="read"} 123`+"\n"+
						`rtmps_conns_outbound_bytes{id="9a07afe4-fc07-4c9b-be6e-6255720c36d0",`+
						`path="mypath",remoteAddr="3.3.3.3:5678",state="read"} 456`+"\n"+
						`rtmps_conns_outbound_frames_discarded{id="9a07afe4-fc07-4c9b-be6e-6255720c36d0",`+
						`path="mypath",remoteAddr="3.3.3.3:5678",state="read"} 12`+"\n"+
						"\n"+
						"# RTMPS connections (deprecated)\n"+
						`rtmps_conns_bytes_received{id="9a07afe4-fc07-4c9b-be6e-6255720c36d0",`+
						`path="mypath",remoteAddr="3.3.3.3:5678",state="read"} 123`+"\n"+
						`rtmps_conns_bytes_sent{id="9a07afe4-fc07-4c9b-be6e-6255720c36d0",`+
						`path="mypath",remoteAddr="3.3.3.3:5678",state="read"} 456`+"\n\n",
					string(byts))

			case "srt_conn":
				require.Equal(t,
					"# SRT connections\n"+ //nolint:dupl
						`srt_conns{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 1`+"\n"+
						`srt_conns_packets_sent{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 100`+"\n"+
						`srt_conns_packets_received{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 200`+"\n"+
						`srt_conns_packets_sent_unique{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 90`+"\n"+
						`srt_conns_packets_received_unique{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 180`+"\n"+
						`srt_conns_packets_send_loss{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 10`+"\n"+
						`srt_conns_packets_received_loss{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 20`+"\n"+
						`srt_conns_packets_retrans{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 5`+"\n"+
						`srt_conns_packets_received_retrans{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 3`+"\n"+
						`srt_conns_packets_sent_ack{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 50`+"\n"+
						`srt_conns_packets_received_ack{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 40`+"\n"+
						`srt_conns_packets_sent_nak{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 2`+"\n"+
						`srt_conns_packets_received_nak{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 1`+"\n"+
						`srt_conns_packets_sent_km{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 4`+"\n"+
						`srt_conns_packets_received_km{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 3`+"\n"+
						`srt_conns_us_snd_duration{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 1000000`+"\n"+
						`srt_conns_packets_received_belated{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 7`+"\n"+
						`srt_conns_packets_send_drop{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 8`+"\n"+
						`srt_conns_packets_received_drop{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 6`+"\n"+
						`srt_conns_packets_received_undecrypt{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 0`+"\n"+
						`srt_conns_bytes_sent{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 12345`+"\n"+
						`srt_conns_bytes_received{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 67890`+"\n"+
						`srt_conns_bytes_sent_unique{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 11000`+"\n"+
						`srt_conns_bytes_received_unique{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 60000`+"\n"+
						`srt_conns_bytes_received_loss{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 500`+"\n"+
						`srt_conns_bytes_retrans{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 300`+"\n"+
						`srt_conns_bytes_received_retrans{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 200`+"\n"+
						`srt_conns_bytes_received_belated{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 100`+"\n"+
						`srt_conns_bytes_send_drop{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 400`+"\n"+
						`srt_conns_bytes_received_drop{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 350`+"\n"+
						`srt_conns_bytes_received_undecrypt{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 0`+"\n"+
						`srt_conns_us_packets_send_period{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 10.5`+"\n"+
						`srt_conns_packets_flow_window{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 8192`+"\n"+
						`srt_conns_packets_flight_size{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 25`+"\n"+
						`srt_conns_ms_rtt{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 1.5`+"\n"+
						`srt_conns_mbps_send_rate{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 50`+"\n"+
						`srt_conns_mbps_receive_rate{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 48`+"\n"+
						`srt_conns_mbps_link_capacity{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 100`+"\n"+
						`srt_conns_bytes_avail_send_buf{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 65536`+"\n"+
						`srt_conns_bytes_avail_receive_buf{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 65536`+"\n"+
						`srt_conns_mbps_max_bw{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 100`+"\n"+
						`srt_conns_bytes_mss{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 1500`+"\n"+
						`srt_conns_packets_send_buf{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 10`+"\n"+
						`srt_conns_bytes_send_buf{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 15000`+"\n"+
						`srt_conns_ms_send_buf{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 120`+"\n"+
						`srt_conns_ms_send_tsb_pd_delay{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 120`+"\n"+
						`srt_conns_packets_receive_buf{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 15`+"\n"+
						`srt_conns_bytes_receive_buf{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 22500`+"\n"+
						`srt_conns_ms_receive_buf{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 120`+"\n"+
						`srt_conns_ms_receive_tsb_pd_delay{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 120`+"\n"+
						`srt_conns_packets_reorder_tolerance{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 3`+"\n"+
						`srt_conns_packets_received_avg_belated_time{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 50`+"\n"+
						`srt_conns_packets_send_loss_rate{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 0.01`+"\n"+
						`srt_conns_packets_received_loss_rate{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 0.02`+"\n"+
						`srt_conns_outbound_frames_discarded{id="a0b1c2d3-e4f5-6789-abcd-ef0123456789",`+
						`path="mypath",remoteAddr="5.5.5.5:4321",state="publish"} 5`+"\n\n",
					string(byts))

			case "webrtc_session":
				require.Equal(t,
					"# WebRTC sessions\n"+
						`webrtc_sessions{id="f47ac10b-58cc-4372-a567-0e02b2c3d479",`+
						`path="mypath",remoteAddr="127.0.0.1:3455",state="read"} 1`+"\n"+
						`webrtc_sessions_inbound_bytes{id="f47ac10b-58cc-4372-a567-0e02b2c3d479",`+
						`path="mypath",remoteAddr="127.0.0.1:3455",state="read"} 123`+"\n"+
						`webrtc_sessions_inbound_rtp_packets{id="f47ac10b-58cc-4372-a567-0e02b2c3d479",`+
						`path="mypath",remoteAddr="127.0.0.1:3455",state="read"} 789`+"\n"+
						`webrtc_sessions_inbound_rtp_packets_lost{id="f47ac10b-58cc-4372-a567-0e02b2c3d479",`+
						`path="mypath",remoteAddr="127.0.0.1:3455",state="read"} 456`+"\n"+
						`webrtc_sessions_inbound_rtp_packets_jitter{id="f47ac10b-58cc-4372-a567-0e02b2c3d479",`+
						`path="mypath",remoteAddr="127.0.0.1:3455",state="read"} 789`+"\n"+
						`webrtc_sessions_inbound_rtcp_packets{id="f47ac10b-58cc-4372-a567-0e02b2c3d479",`+
						`path="mypath",remoteAddr="127.0.0.1:3455",state="read"} 123`+"\n"+
						`webrtc_sessions_outbound_bytes{id="f47ac10b-58cc-4372-a567-0e02b2c3d479",`+
						`path="mypath",remoteAddr="127.0.0.1:3455",state="read"} 456`+"\n"+
						`webrtc_sessions_outbound_rtp_packets{id="f47ac10b-58cc-4372-a567-0e02b2c3d479",`+
						`path="mypath",remoteAddr="127.0.0.1:3455",state="read"} 123`+"\n"+
						`webrtc_sessions_outbound_rtcp_packets{id="f47ac10b-58cc-4372-a567-0e02b2c3d479",`+
						`path="mypath",remoteAddr="127.0.0.1:3455",state="read"} 456`+"\n"+
						`webrtc_sessions_outbound_frames_discarded{id="f47ac10b-58cc-4372-a567-0e02b2c3d479",`+
						`path="mypath",remoteAddr="127.0.0.1:3455",state="read"} 12`+"\n"+
						"\n"+
						"# WebRTC sessions (deprecated)\n"+
						`webrtc_sessions_bytes_received{id="f47ac10b-58cc-4372-a567-0e02b2c3d479",`+
						`path="mypath",remoteAddr="127.0.0.1:3455",state="read"} 123`+"\n"+
						`webrtc_sessions_bytes_sent{id="f47ac10b-58cc-4372-a567-0e02b2c3d479",`+
						`path="mypath",remoteAddr="127.0.0.1:3455",state="read"} 456`+"\n"+
						`webrtc_sessions_rtp_packets_received{id="f47ac10b-58cc-4372-a567-0e02b2c3d479",`+
						`path="mypath",remoteAddr="127.0.0.1:3455",state="read"} 789`+"\n"+
						`webrtc_sessions_rtp_packets_sent{id="f47ac10b-58cc-4372-a567-0e02b2c3d479",`+
						`path="mypath",remoteAddr="127.0.0.1:3455",state="read"} 123`+"\n"+
						`webrtc_sessions_rtp_packets_lost{id="f47ac10b-58cc-4372-a567-0e02b2c3d479",`+
						`path="mypath",remoteAddr="127.0.0.1:3455",state="read"} 456`+"\n"+
						`webrtc_sessions_rtp_packets_jitter{id="f47ac10b-58cc-4372-a567-0e02b2c3d479",`+
						`path="mypath",remoteAddr="127.0.0.1:3455",state="read"} 789`+"\n"+
						`webrtc_sessions_rtcp_packets_received{id="f47ac10b-58cc-4372-a567-0e02b2c3d479",`+
						`path="mypath",remoteAddr="127.0.0.1:3455",state="read"} 123`+"\n"+
						`webrtc_sessions_rtcp_packets_sent{id="f47ac10b-58cc-4372-a567-0e02b2c3d479",`+
						`path="mypath",remoteAddr="127.0.0.1:3455",state="read"} 456`+"\n\n",
					string(byts))
			}
		})
	}
}

func TestFilterByType(t *testing.T) {
	for _, ca := range []string{"paths", "rtmp_conns", "rtmps_conns", "webrtc_sessions"} {
		t.Run(ca, func(t *testing.T) {
			m := Metrics{
				Address:      "localhost:9998",
				AllowOrigins: []string{"*"},
				ReadTimeout:  conf.Duration(10 * time.Second),
				WriteTimeout: conf.Duration(10 * time.Second),
				AuthManager:  test.NilAuthManager,
				Parent:       test.NilLogger,
			}
			err := m.Initialize()
			require.NoError(t, err)
			defer m.Close()

			m.SetPathManager(&dummyPathManager{})
			m.SetHLSServer(&dummyHLSServer{})
			m.SetRTSPServer(&dummyRTSPServer{})
			m.SetRTSPSServer(&dummyRTSPServer{})
			m.SetRTMPServer(&dummyRTMPServer{})
			m.SetRTMPSServer(&dummyRTMPServer{})
			m.SetSRTServer(&dummySRTServer{})
			m.SetWebRTCServer(&dummyWebRTCServer{})

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr}

			var query string
			switch ca {
			case "paths":
				query = "type=paths"

			case "rtmp_conns":
				query = "type=rtmp_conns"

			case "rtmps_conns":
				query = "type=rtmps_conns"

			case "webrtc_sessions":
				query = "type=webrtc_sessions"
			}

			res, err := hc.Get("http://localhost:9998/metrics?" + query)
			require.NoError(t, err)
			defer res.Body.Close()

			byts, err := io.ReadAll(res.Body)
			require.NoError(t, err)

			switch ca {
			case "paths":
				require.Equal(t,
					"# Paths\n"+
						"paths{name=\"mypath\",state=\"ready\"} 1\n"+
						"paths_readers{name=\"mypath\",readerType=\"rtmpConn\",state=\"ready\"} 1\n"+
						"paths_readers{name=\"mypath\",readerType=\"rtspSession\",state=\"ready\"} 2\n"+
						"paths_inbound_bytes{name=\"mypath\",state=\"ready\"} 123\n"+
						"paths_outbound_bytes{name=\"mypath\",state=\"ready\"} 456\n"+
						"paths_inbound_frames_in_error{name=\"mypath\",state=\"ready\"} 7\n"+
						"\n"+
						"# Paths (deprecated)\n"+
						"paths_bytes_received{name=\"mypath\",state=\"ready\"} 123\n"+
						"paths_bytes_sent{name=\"mypath\",state=\"ready\"} 456\n\n",
					string(byts))

			case "rtmp_conns":
				require.Equal(t,
					"# RTMP connections\n"+
						"rtmp_conns{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
						"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 1\n"+
						"rtmp_conns_inbound_bytes{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
						"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 123\n"+
						"rtmp_conns_outbound_bytes{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
						"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 456\n"+
						"rtmp_conns_outbound_frames_discarded{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
						"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 12\n"+
						"\n"+
						"# RTMP connections (deprecated)\n"+
						"rtmp_conns_bytes_received{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
						"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 123\n"+
						"rtmp_conns_bytes_sent{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
						"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 456\n\n",
					string(byts))

			case "rtmps_conns":
				require.Equal(t,
					"# RTMPS connections\n"+
						"rtmps_conns{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
						"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 1\n"+
						"rtmps_conns_inbound_bytes{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
						"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 123\n"+
						"rtmps_conns_outbound_bytes{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
						"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 456\n"+
						"rtmps_conns_outbound_frames_discarded{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
						"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 12\n"+
						"\n"+
						"# RTMPS connections (deprecated)\n"+
						"rtmps_conns_bytes_received{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
						"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 123\n"+
						"rtmps_conns_bytes_sent{id=\"9a07afe4-fc07-4c9b-be6e-6255720c36d0\",path=\"mypath\","+
						"remoteAddr=\"3.3.3.3:5678\",state=\"read\"} 456\n\n",
					string(byts))

			case "webrtc_sessions":
				require.Equal(t,
					"# WebRTC sessions\n"+
						"webrtc_sessions{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
						"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 1\n"+
						"webrtc_sessions_inbound_bytes{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
						"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 123\n"+
						"webrtc_sessions_inbound_rtp_packets{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
						"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 789\n"+
						"webrtc_sessions_inbound_rtp_packets_lost{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
						"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 456\n"+
						"webrtc_sessions_inbound_rtp_packets_jitter{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
						"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 789\n"+
						"webrtc_sessions_inbound_rtcp_packets{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
						"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 123\n"+
						"webrtc_sessions_outbound_bytes{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
						"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 456\n"+
						"webrtc_sessions_outbound_rtp_packets{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
						"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 123\n"+
						"webrtc_sessions_outbound_rtcp_packets{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
						"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 456\n"+
						"webrtc_sessions_outbound_frames_discarded{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
						"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 12\n"+
						"\n"+
						"# WebRTC sessions (deprecated)\n"+
						"webrtc_sessions_bytes_received{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
						"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 123\n"+
						"webrtc_sessions_bytes_sent{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
						"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 456\n"+
						"webrtc_sessions_rtp_packets_received{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
						"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 789\n"+
						"webrtc_sessions_rtp_packets_sent{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
						"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 123\n"+
						"webrtc_sessions_rtp_packets_lost{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
						"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 456\n"+
						"webrtc_sessions_rtp_packets_jitter{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
						"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 789\n"+
						"webrtc_sessions_rtcp_packets_received{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
						"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 123\n"+
						"webrtc_sessions_rtcp_packets_sent{id=\"f47ac10b-58cc-4372-a567-0e02b2c3d479\",path=\"mypath\","+
						"remoteAddr=\"127.0.0.1:3455\",state=\"read\"} 456\n\n",
					string(byts))
			}
		})
	}
}

func TestAuthError(t *testing.T) {
	n := 0

	m := Metrics{
		Address:      "localhost:9998",
		AllowOrigins: []string{"*"},
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager: &test.AuthManager{
			AuthenticateImpl: func(req *auth.Request) (string, *auth.Error) {
				if req.Credentials.User == "" {
					return "", &auth.Error{AskCredentials: true, Wrapped: fmt.Errorf("auth error")}
				}
				return "", &auth.Error{Wrapped: fmt.Errorf("auth error")}
			},
		},
		Parent: test.Logger(func(l logger.Level, s string, i ...any) {
			if l == logger.Info {
				if n == 1 {
					require.Regexp(t, "failed to authenticate: auth error$", fmt.Sprintf(s, i...))
				}
				n++
			}
		}),
	}
	err := m.Initialize()
	require.NoError(t, err)
	defer m.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	res, err := hc.Get("http://localhost:9998/metrics")
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	require.Equal(t, `Basic realm="mediamtx"`, res.Header.Get("WWW-Authenticate"))

	res, err = hc.Get("http://myuser:mypass@localhost:9998/metrics")
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusUnauthorized, res.StatusCode)

	require.Equal(t, 2, n)
}

func TestMetricsConcurrentSettersAndReads(t *testing.T) {
	m := Metrics{
		Address:      "localhost:9998",
		AllowOrigins: []string{"*"},
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		Parent:       test.NilLogger,
	}
	err := m.Initialize()
	require.NoError(t, err)
	defer m.Close()

	m.SetPathManager(&dummyPathManager{})
	m.SetHLSServer(&dummyHLSServer{})
	m.SetRTSPServer(&dummyRTSPServer{})
	m.SetRTSPSServer(&dummyRTSPServer{})
	m.SetRTMPServer(&dummyRTMPServer{})
	m.SetRTMPSServer(&dummyRTMPServer{})
	m.SetSRTServer(&dummySRTServer{})
	m.SetWebRTCServer(&dummyWebRTCServer{})

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var readers sync.WaitGroup
	for range 8 {
		readers.Go(func() {
			for range 100 {
				res, err2 := hc.Get("http://localhost:9998/metrics")
				require.NoError(t, err2)
				_, err2 = io.Copy(io.Discard, res.Body)
				res.Body.Close()
				require.NoError(t, err2)
			}
		})
	}

	for range 1000 {
		m.SetPathManager(&dummyPathManager{})
		m.SetHLSServer(&dummyHLSServer{})
		m.SetRTSPServer(&dummyRTSPServer{})
		m.SetRTSPSServer(&dummyRTSPServer{})
		m.SetRTMPServer(&dummyRTMPServer{})
		m.SetRTMPSServer(&dummyRTMPServer{})
		m.SetSRTServer(&dummySRTServer{})
		m.SetWebRTCServer(&dummyWebRTCServer{})

		m.SetHLSServer(nil)
		m.SetRTSPServer(nil)
		m.SetRTSPSServer(nil)
		m.SetRTMPServer(nil)
		m.SetRTMPSServer(nil)
		m.SetSRTServer(nil)
		m.SetWebRTCServer(nil)
	}

	readers.Wait()
}

func BenchmarkTags(b *testing.B) {
	m := map[string]string{
		"id":         "124b22ce-9c34-4387-b045-44caf98049f7",
		"state":      "publish",
		"path":       "mypath",
		"remoteAddr": "124.5.5.5:34542",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tags(m)
	}
}

func BenchmarkMetric(b *testing.B) {
	ta := tags(map[string]string{
		"id":         "124b22ce-9c34-4387-b045-44caf98049f7",
		"state":      "publish",
		"path":       "mypath",
		"remoteAddr": "124.5.5.5:34542",
	})
	var out strings.Builder
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out.Reset()
		metric(&out, "rtsp_sessions_inbound_bytes", ta, 123)
	}
}

func BenchmarkFullMetricsHandler(b *testing.B) {
	m := Metrics{
		Address:      "localhost:9998",
		AllowOrigins: []string{"*"},
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		Parent:       test.NilLogger,
	}
	err := m.Initialize()
	if err != nil {
		b.Fatal(err)
	}
	defer m.Close()

	m.SetPathManager(&dummyPathManager{})
	m.SetHLSServer(&dummyHLSServer{})
	m.SetRTSPServer(&dummyRTSPServer{})
	m.SetRTSPSServer(&dummyRTSPServer{})
	m.SetRTMPServer(&dummyRTMPServer{})
	m.SetRTMPSServer(&dummyRTMPServer{})
	m.SetSRTServer(&dummySRTServer{})
	m.SetWebRTCServer(&dummyWebRTCServer{})

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var res *http.Response
		res, err = hc.Get("http://localhost:9998/metrics")
		if err != nil {
			b.Fatal(err)
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
	}
}
