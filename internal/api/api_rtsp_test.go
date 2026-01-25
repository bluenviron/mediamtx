package api //nolint:revive

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/servers/rtsp"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type testRTSPServer struct {
	conns    map[uuid.UUID]*defs.APIRTSPConn
	sessions map[uuid.UUID]*defs.APIRTSPSession
}

func (s *testRTSPServer) APIConnsList() (*defs.APIRTSPConnsList, error) {
	items := make([]defs.APIRTSPConn, 0, len(s.conns))
	for _, conn := range s.conns {
		items = append(items, *conn)
	}
	return &defs.APIRTSPConnsList{Items: items}, nil
}

func (s *testRTSPServer) APIConnsGet(id uuid.UUID) (*defs.APIRTSPConn, error) {
	conn, ok := s.conns[id]
	if !ok {
		return nil, rtsp.ErrConnNotFound
	}
	return conn, nil
}

func (s *testRTSPServer) APISessionsList() (*defs.APIRTSPSessionList, error) {
	items := make([]defs.APIRTSPSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		items = append(items, *session)
	}
	return &defs.APIRTSPSessionList{Items: items}, nil
}

func (s *testRTSPServer) APISessionsGet(id uuid.UUID) (*defs.APIRTSPSession, error) {
	session, ok := s.sessions[id]
	if !ok {
		return nil, rtsp.ErrSessionNotFound
	}
	return session, nil
}

func (s *testRTSPServer) APISessionsKick(id uuid.UUID) error {
	_, ok := s.sessions[id]
	if !ok {
		return rtsp.ErrSessionNotFound
	}
	return nil
}

func TestRTSPConnsList(t *testing.T) {
	for _, ca := range []struct {
		name     string
		endpoint string
		secure   bool
	}{
		{
			name:     "rtsp",
			endpoint: "rtspconns",
			secure:   false,
		},
		{
			name:     "rtsps",
			endpoint: "rtspsconns",
			secure:   true,
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			id1 := uuid.New()
			id2 := uuid.New()
			sessionID := uuid.New()
			now := time.Now()

			rtspServer := &testRTSPServer{
				conns: map[uuid.UUID]*defs.APIRTSPConn{
					id1: {
						ID:            id1,
						Created:       now,
						RemoteAddr:    "192.168.1.1:5000",
						BytesReceived: 1000,
						BytesSent:     2000,
						Session:       &sessionID,
						Tunnel:        "",
					},
					id2: {
						ID:            id2,
						Created:       now.Add(time.Minute),
						RemoteAddr:    "192.168.1.2:5001",
						BytesReceived: 500,
						BytesSent:     1500,
						Session:       nil,
						Tunnel:        "http",
					},
				},
			}

			api := API{
				Address:      "localhost:9997",
				ReadTimeout:  conf.Duration(10 * time.Second),
				WriteTimeout: conf.Duration(10 * time.Second),
				AuthManager:  test.NilAuthManager,
				Parent:       &testParent{},
			}
			if ca.secure {
				api.RTSPSServer = rtspServer
			} else {
				api.RTSPServer = rtspServer
			}
			err := api.Initialize()
			require.NoError(t, err)
			defer api.Close()

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr}

			req, err := http.NewRequest(http.MethodGet,
				fmt.Sprintf("http://localhost:9997/v3/%s/list", ca.endpoint), nil)
			require.NoError(t, err)

			res, err := hc.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			require.Equal(t, http.StatusOK, res.StatusCode)

			var out defs.APIRTSPConnsList
			err = json.NewDecoder(res.Body).Decode(&out)
			require.NoError(t, err)

			require.Equal(t, 2, out.ItemCount)
			require.Equal(t, 1, out.PageCount)
			require.Len(t, out.Items, 2)
		})
	}
}

func TestRTSPConnsGet(t *testing.T) {
	for _, ca := range []struct {
		name     string
		endpoint string
		secure   bool
	}{
		{
			name:     "rtsp",
			endpoint: "rtspconns",
			secure:   false,
		},
		{
			name:     "rtsps",
			endpoint: "rtspsconns",
			secure:   true,
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			id := uuid.New()
			sessionID := uuid.New()
			now := time.Now()

			rtspServer := &testRTSPServer{
				conns: map[uuid.UUID]*defs.APIRTSPConn{
					id: {
						ID:            id,
						Created:       now,
						RemoteAddr:    "192.168.1.100:5000",
						BytesReceived: 999999,
						BytesSent:     888888,
						Session:       &sessionID,
						Tunnel:        "",
					},
				},
			}

			api := API{
				Address:      "localhost:9997",
				ReadTimeout:  conf.Duration(10 * time.Second),
				WriteTimeout: conf.Duration(10 * time.Second),
				AuthManager:  test.NilAuthManager,
				Parent:       &testParent{},
			}
			if ca.secure {
				api.RTSPSServer = rtspServer
			} else {
				api.RTSPServer = rtspServer
			}
			err := api.Initialize()
			require.NoError(t, err)
			defer api.Close()

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr}

			req, err := http.NewRequest(http.MethodGet,
				fmt.Sprintf("http://localhost:9997/v3/%s/get/%s", ca.endpoint, id), nil)
			require.NoError(t, err)

			res, err := hc.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			require.Equal(t, http.StatusOK, res.StatusCode)

			var out defs.APIRTSPConn
			err = json.NewDecoder(res.Body).Decode(&out)
			require.NoError(t, err)

			require.Equal(t, id, out.ID)
			require.Equal(t, "192.168.1.100:5000", out.RemoteAddr)
			require.Equal(t, uint64(999999), out.BytesReceived)
			require.NotNil(t, out.Session)
			require.Equal(t, sessionID, *out.Session)
		})
	}
}

func TestRTSPSessionsList(t *testing.T) {
	for _, ca := range []struct {
		name     string
		endpoint string
		secure   bool
	}{
		{
			name:     "rtsp",
			endpoint: "rtspsessions",
			secure:   false,
		},
		{
			name:     "rtsps",
			endpoint: "rtspssessions",
			secure:   true,
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			id1 := uuid.New()
			id2 := uuid.New()
			now := time.Now()
			transport := "UDP"
			profile := "AVP"

			rtspServer := &testRTSPServer{
				sessions: map[uuid.UUID]*defs.APIRTSPSession{
					id1: {
						ID:                  id1,
						Created:             now,
						RemoteAddr:          "192.168.1.1:5000",
						State:               defs.APIRTSPSessionStatePublish,
						Path:                "stream1",
						Query:               "token=abc",
						Transport:           &transport,
						Profile:             &profile,
						BytesReceived:       1000,
						BytesSent:           2000,
						RTPPacketsReceived:  100,
						RTPPacketsSent:      200,
						RTPPacketsLost:      5,
						RTPPacketsInError:   2,
						RTPPacketsJitter:    0.5,
						RTCPPacketsReceived: 10,
						RTCPPacketsSent:     15,
						RTCPPacketsInError:  1,
					},
					id2: {
						ID:                  id2,
						Created:             now.Add(time.Minute),
						RemoteAddr:          "192.168.1.2:5001",
						State:               defs.APIRTSPSessionStateRead,
						Path:                "stream2",
						Query:               "",
						Transport:           nil,
						Profile:             nil,
						BytesReceived:       500,
						BytesSent:           1500,
						RTPPacketsReceived:  50,
						RTPPacketsSent:      150,
						RTPPacketsLost:      0,
						RTPPacketsInError:   0,
						RTPPacketsJitter:    0.1,
						RTCPPacketsReceived: 5,
						RTCPPacketsSent:     10,
						RTCPPacketsInError:  0,
					},
				},
			}

			api := API{
				Address:      "localhost:9997",
				ReadTimeout:  conf.Duration(10 * time.Second),
				WriteTimeout: conf.Duration(10 * time.Second),
				AuthManager:  test.NilAuthManager,
				Parent:       &testParent{},
			}
			if ca.secure {
				api.RTSPSServer = rtspServer
			} else {
				api.RTSPServer = rtspServer
			}
			err := api.Initialize()
			require.NoError(t, err)
			defer api.Close()

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr}

			req, err := http.NewRequest(http.MethodGet,
				fmt.Sprintf("http://localhost:9997/v3/%s/list", ca.endpoint), nil)
			require.NoError(t, err)

			res, err := hc.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			require.Equal(t, http.StatusOK, res.StatusCode)

			var out defs.APIRTSPSessionList
			err = json.NewDecoder(res.Body).Decode(&out)
			require.NoError(t, err)

			require.Equal(t, 2, out.ItemCount)
			require.Equal(t, 1, out.PageCount)
			require.Len(t, out.Items, 2)
		})
	}
}

func TestRTSPSessionsGet(t *testing.T) {
	for _, ca := range []struct {
		name     string
		endpoint string
		secure   bool
	}{
		{
			name:     "rtsp",
			endpoint: "rtspsessions",
			secure:   false,
		},
		{
			name:     "rtsps",
			endpoint: "rtspssessions",
			secure:   true,
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			id := uuid.New()
			now := time.Now()
			transport := "UDP"
			profile := "AVP"

			rtspServer := &testRTSPServer{
				sessions: map[uuid.UUID]*defs.APIRTSPSession{
					id: {
						ID:                  id,
						Created:             now,
						RemoteAddr:          "192.168.1.100:5000",
						State:               defs.APIRTSPSessionStatePublish,
						Path:                "mystream",
						Query:               "key=value",
						Transport:           &transport,
						Profile:             &profile,
						BytesReceived:       999999,
						BytesSent:           888888,
						RTPPacketsReceived:  10000,
						RTPPacketsSent:      20000,
						RTPPacketsLost:      50,
						RTPPacketsInError:   10,
						RTPPacketsJitter:    1.5,
						RTCPPacketsReceived: 100,
						RTCPPacketsSent:     200,
						RTCPPacketsInError:  5,
					},
				},
			}

			api := API{
				Address:      "localhost:9997",
				ReadTimeout:  conf.Duration(10 * time.Second),
				WriteTimeout: conf.Duration(10 * time.Second),
				AuthManager:  test.NilAuthManager,
				Parent:       &testParent{},
			}
			if ca.secure {
				api.RTSPSServer = rtspServer
			} else {
				api.RTSPServer = rtspServer
			}
			err := api.Initialize()
			require.NoError(t, err)
			defer api.Close()

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr}

			req, err := http.NewRequest(http.MethodGet,
				fmt.Sprintf("http://localhost:9997/v3/%s/get/%s", ca.endpoint, id), nil)
			require.NoError(t, err)

			res, err := hc.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			require.Equal(t, http.StatusOK, res.StatusCode)

			var out defs.APIRTSPSession
			err = json.NewDecoder(res.Body).Decode(&out)
			require.NoError(t, err)

			require.Equal(t, id, out.ID)
			require.Equal(t, "192.168.1.100:5000", out.RemoteAddr)
			require.Equal(t, defs.APIRTSPSessionStatePublish, out.State)
			require.Equal(t, "mystream", out.Path)
			require.Equal(t, uint64(999999), out.BytesReceived)
			require.NotNil(t, out.Transport)
			require.Equal(t, "UDP", *out.Transport)
		})
	}
}

func TestRTSPSessionsKick(t *testing.T) {
	for _, ca := range []struct {
		name     string
		endpoint string
		secure   bool
	}{
		{
			name:     "rtsp",
			endpoint: "rtspsessions",
			secure:   false,
		},
		{
			name:     "rtsps",
			endpoint: "rtspssessions",
			secure:   true,
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			id := uuid.New()
			now := time.Now()
			transport := "UDP"
			profile := "AVP"

			rtspServer := &testRTSPServer{
				sessions: map[uuid.UUID]*defs.APIRTSPSession{
					id: {
						ID:                  id,
						Created:             now,
						RemoteAddr:          "192.168.1.100:5000",
						State:               defs.APIRTSPSessionStatePublish,
						Path:                "mystream",
						Query:               "",
						Transport:           &transport,
						Profile:             &profile,
						BytesReceived:       1000,
						BytesSent:           2000,
						RTPPacketsReceived:  100,
						RTPPacketsSent:      200,
						RTPPacketsLost:      0,
						RTPPacketsInError:   0,
						RTPPacketsJitter:    0.5,
						RTCPPacketsReceived: 10,
						RTCPPacketsSent:     15,
						RTCPPacketsInError:  0,
					},
				},
			}

			api := API{
				Address:      "localhost:9997",
				ReadTimeout:  conf.Duration(10 * time.Second),
				WriteTimeout: conf.Duration(10 * time.Second),
				AuthManager:  test.NilAuthManager,
				Parent:       &testParent{},
			}
			if ca.secure {
				api.RTSPSServer = rtspServer
			} else {
				api.RTSPServer = rtspServer
			}
			err := api.Initialize()
			require.NoError(t, err)
			defer api.Close()

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr}

			req, err := http.NewRequest(http.MethodPost,
				fmt.Sprintf("http://localhost:9997/v3/%s/kick/%s", ca.endpoint, id), nil)
			require.NoError(t, err)

			res, err := hc.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			require.Equal(t, http.StatusOK, res.StatusCode)
			checkOK(t, res.Body)
		})
	}
}
