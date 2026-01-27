package api //nolint:revive

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/servers/webrtc"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type testWebRTCServer struct {
	sessions map[uuid.UUID]*defs.APIWebRTCSession
}

func (s *testWebRTCServer) APISessionsList() (*defs.APIWebRTCSessionList, error) {
	items := make([]defs.APIWebRTCSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		items = append(items, *session)
	}
	return &defs.APIWebRTCSessionList{Items: items}, nil
}

func (s *testWebRTCServer) APISessionsGet(id uuid.UUID) (*defs.APIWebRTCSession, error) {
	session, ok := s.sessions[id]
	if !ok {
		return nil, webrtc.ErrSessionNotFound
	}
	return session, nil
}

func (s *testWebRTCServer) APISessionsKick(id uuid.UUID) error {
	_, ok := s.sessions[id]
	if !ok {
		return webrtc.ErrSessionNotFound
	}
	return nil
}

func TestWebRTCSessionsList(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	now := time.Now()

	webrtcServer := &testWebRTCServer{
		sessions: map[uuid.UUID]*defs.APIWebRTCSession{
			id1: {
				ID:                        id1,
				Created:                   now,
				RemoteAddr:                "192.168.1.1:5000",
				PeerConnectionEstablished: true,
				LocalCandidate:            "192.168.1.100:8000",
				RemoteCandidate:           "192.168.1.1:5000",
				State:                     defs.APIWebRTCSessionStatePublish,
				Path:                      "stream1",
				Query:                     "token=abc",
				BytesReceived:             1000,
				BytesSent:                 2000,
				RTPPacketsReceived:        100,
				RTPPacketsSent:            200,
				RTPPacketsLost:            5,
				RTPPacketsJitter:          0.5,
				RTCPPacketsReceived:       10,
				RTCPPacketsSent:           15,
			},
			id2: {
				ID:                        id2,
				Created:                   now.Add(time.Minute),
				RemoteAddr:                "192.168.1.2:5001",
				PeerConnectionEstablished: true,
				LocalCandidate:            "192.168.1.100:8001",
				RemoteCandidate:           "192.168.1.2:5001",
				State:                     defs.APIWebRTCSessionStateRead,
				Path:                      "stream2",
				Query:                     "",
				BytesReceived:             500,
				BytesSent:                 1500,
				RTPPacketsReceived:        50,
				RTPPacketsSent:            150,
				RTPPacketsLost:            0,
				RTPPacketsJitter:          0.1,
				RTCPPacketsReceived:       5,
				RTCPPacketsSent:           10,
			},
		},
	}

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		WebRTCServer: webrtcServer,
		Parent:       &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var out defs.APIWebRTCSessionList
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/webrtcsessions/list", nil, &out)

	require.Equal(t, 2, out.ItemCount)
	require.Equal(t, 1, out.PageCount)
	require.Len(t, out.Items, 2)
}

func TestWebRTCSessionsGet(t *testing.T) {
	id := uuid.New()
	now := time.Now()

	webrtcServer := &testWebRTCServer{
		sessions: map[uuid.UUID]*defs.APIWebRTCSession{
			id: {
				ID:                        id,
				Created:                   now,
				RemoteAddr:                "192.168.1.100:5000",
				PeerConnectionEstablished: true,
				LocalCandidate:            "192.168.1.200:8000",
				RemoteCandidate:           "192.168.1.100:5000",
				State:                     defs.APIWebRTCSessionStatePublish,
				Path:                      "mystream",
				Query:                     "key=value",
				BytesReceived:             999999,
				BytesSent:                 888888,
				RTPPacketsReceived:        10000,
				RTPPacketsSent:            20000,
				RTPPacketsLost:            50,
				RTPPacketsJitter:          1.5,
				RTCPPacketsReceived:       100,
				RTCPPacketsSent:           200,
			},
		},
	}

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		WebRTCServer: webrtcServer,
		Parent:       &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var out defs.APIWebRTCSession
	httpRequest(t, hc, http.MethodGet, fmt.Sprintf("http://localhost:9997/v3/webrtcsessions/get/%s", id), nil, &out)

	require.Equal(t, id, out.ID)
	require.Equal(t, "192.168.1.100:5000", out.RemoteAddr)
	require.Equal(t, defs.APIWebRTCSessionStatePublish, out.State)
	require.Equal(t, "mystream", out.Path)
	require.True(t, out.PeerConnectionEstablished)
	require.Equal(t, "192.168.1.200:8000", out.LocalCandidate)
	require.Equal(t, "192.168.1.100:5000", out.RemoteCandidate)
	require.Equal(t, uint64(999999), out.BytesReceived)
	require.Equal(t, uint64(888888), out.BytesSent)
	require.Equal(t, uint64(10000), out.RTPPacketsReceived)
	require.Equal(t, uint64(20000), out.RTPPacketsSent)
	require.Equal(t, uint64(50), out.RTPPacketsLost)
	require.Equal(t, 1.5, out.RTPPacketsJitter)
}

func TestWebRTCSessionsKick(t *testing.T) {
	id := uuid.New()
	now := time.Now()

	webrtcServer := &testWebRTCServer{
		sessions: map[uuid.UUID]*defs.APIWebRTCSession{
			id: {
				ID:                        id,
				Created:                   now,
				RemoteAddr:                "192.168.1.100:5000",
				PeerConnectionEstablished: true,
				LocalCandidate:            "192.168.1.200:8000",
				RemoteCandidate:           "192.168.1.100:5000",
				State:                     defs.APIWebRTCSessionStatePublish,
				Path:                      "mystream",
				Query:                     "",
				BytesReceived:             1000,
				BytesSent:                 2000,
				RTPPacketsReceived:        100,
				RTPPacketsSent:            200,
				RTPPacketsLost:            0,
				RTPPacketsJitter:          0.5,
				RTCPPacketsReceived:       10,
				RTCPPacketsSent:           15,
			},
		},
	}

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		WebRTCServer: webrtcServer,
		Parent:       &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	httpRequest(t, hc, http.MethodPost, fmt.Sprintf("http://localhost:9997/v3/webrtcsessions/kick/%s", id), nil, nil)
}
