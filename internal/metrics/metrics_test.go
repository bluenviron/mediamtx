package metrics

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func timePtr(t time.Time) *time.Time {
	return &t
}

type dummyPathManager struct{}

func (dummyPathManager) APIPathsList() (*defs.APIPathList, error) {
	return &defs.APIPathList{
		ItemCount: 1,
		PageCount: 1,
		Items: []*defs.APIPath{{
			Name:     "mypath",
			ConfName: "mypathconf",
			Source: &defs.APIPathSourceOrReader{
				Type: "testing",
				ID:   "123324354",
			},
			Ready:         true,
			ReadyTime:     timePtr(time.Date(2003, 11, 4, 23, 15, 7, 0, time.UTC)),
			Tracks:        []string{"H264", "H265"},
			BytesReceived: 123,
			BytesSent:     456,
			Readers: []defs.APIPathSourceOrReader{
				{
					Type: "testing",
					ID:   "345234423",
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
		Items: []*defs.APIHLSMuxer{{
			Path:        "mypath",
			Created:     time.Date(2003, 11, 4, 23, 15, 7, 0, time.UTC),
			LastRequest: time.Date(2003, 11, 4, 23, 15, 7, 0, time.UTC),
			BytesSent:   789,
		}},
	}, nil
}

func (dummyHLSServer) APIMuxersGet(string) (*defs.APIHLSMuxer, error) {
	panic("unused")
}

type dummyWebRTCServer struct{}

func (dummyWebRTCServer) APISessionsList() (*defs.APIWebRTCSessionList, error) {
	return &defs.APIWebRTCSessionList{
		ItemCount: 1,
		PageCount: 1,
		Items: []*defs.APIWebRTCSession{{
			ID:                        uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
			Created:                   time.Date(2003, 11, 4, 23, 15, 7, 0, time.UTC),
			RemoteAddr:                "127.0.0.1:3455",
			PeerConnectionEstablished: true,
			LocalCandidate:            "local",
			RemoteCandidate:           "remote",
			State:                     defs.APIWebRTCSessionStateRead,
			Path:                      "mypath",
			Query:                     "myquery",
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

func TestPreflightRequest(t *testing.T) {
	m := Metrics{
		Address:     "localhost:9998",
		AllowOrigin: "*",
		ReadTimeout: conf.Duration(10 * time.Second),
		AuthManager: test.NilAuthManager,
		Parent:      test.NilLogger,
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
	m := Metrics{
		Address:     "localhost:9998",
		AllowOrigin: "*",
		ReadTimeout: conf.Duration(10 * time.Second),
		AuthManager: test.NilAuthManager,
		Parent:      test.NilLogger,
	}
	err := m.Initialize()
	require.NoError(t, err)
	defer m.Close()

	m.SetPathManager(&dummyPathManager{})
	m.SetHLSServer(&dummyHLSServer{})
	m.SetWebRTCServer(&dummyWebRTCServer{})

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	res, err := hc.Get("http://localhost:9998/metrics")
	require.NoError(t, err)
	defer res.Body.Close()

	byts, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	require.Equal(t,
		`paths{name="mypath",state="ready"} 1`+"\n"+
			`paths_bytes_received{name="mypath",state="ready"} 123`+"\n"+
			`paths_bytes_sent{name="mypath",state="ready"} 456`+"\n"+
			`paths_readers{name="mypath",state="ready"} 1`+"\n"+
			`hls_muxers{name="mypath"} 1`+"\n"+
			`hls_muxers_bytes_sent{name="mypath"} 789`+"\n"+
			`webrtc_sessions{id="f47ac10b-58cc-4372-a567-0e02b2c3d479",`+
			`path="mypath",remoteAddr="127.0.0.1:3455",state="read"} 1`+"\n"+
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
			`path="mypath",remoteAddr="127.0.0.1:3455",state="read"} 456`+"\n",
		string(byts))
}
