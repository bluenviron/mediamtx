package api //nolint:revive

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/servers/rtmp"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type testRTMPServer struct {
	conns map[uuid.UUID]*defs.APIRTMPConn
}

func (s *testRTMPServer) APIConnsList() (*defs.APIRTMPConnList, error) {
	items := make([]defs.APIRTMPConn, 0, len(s.conns))
	for _, conn := range s.conns {
		items = append(items, *conn)
	}
	return &defs.APIRTMPConnList{Items: items}, nil
}

func (s *testRTMPServer) APIConnsGet(id uuid.UUID) (*defs.APIRTMPConn, error) {
	conn, ok := s.conns[id]
	if !ok {
		return nil, rtmp.ErrConnNotFound
	}
	return conn, nil
}

func (s *testRTMPServer) APIConnsKick(id uuid.UUID) error {
	_, ok := s.conns[id]
	if !ok {
		return rtmp.ErrConnNotFound
	}
	return nil
}

func TestRTMPConnsList(t *testing.T) {
	for _, ca := range []struct {
		name     string
		endpoint string
		isSecure bool
	}{
		{
			name:     "rtmp",
			endpoint: "rtmpconns",
			isSecure: false,
		},
		{
			name:     "rtmps",
			endpoint: "rtmpsconns",
			isSecure: true,
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			id1 := uuid.New()
			id2 := uuid.New()
			now := time.Now()

			rtmpServer := &testRTMPServer{
				conns: map[uuid.UUID]*defs.APIRTMPConn{
					id1: {
						ID:            id1,
						Created:       now,
						RemoteAddr:    "192.168.1.1:5000",
						State:         defs.APIRTMPConnStatePublish,
						Path:          "stream1",
						Query:         "token=abc",
						BytesReceived: 1000,
						BytesSent:     2000,
					},
					id2: {
						ID:            id2,
						Created:       now.Add(time.Minute),
						RemoteAddr:    "192.168.1.2:5001",
						State:         defs.APIRTMPConnStateRead,
						Path:          "stream2",
						Query:         "",
						BytesReceived: 500,
						BytesSent:     1500,
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

			if ca.isSecure {
				api.RTMPSServer = rtmpServer
			} else {
				api.RTMPServer = rtmpServer
			}

			err := api.Initialize()
			require.NoError(t, err)
			defer api.Close()

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr}

			var out defs.APIRTMPConnList
			httpRequest(t, hc, http.MethodGet, fmt.Sprintf("http://localhost:9997/v3/%s/list", ca.endpoint), nil, &out)

			require.Equal(t, 2, out.ItemCount)
			require.Equal(t, 1, out.PageCount)
			require.Len(t, out.Items, 2)
		})
	}
}

func TestRTMPConnsGet(t *testing.T) {
	for _, ca := range []struct {
		name     string
		endpoint string
		path     string
		isSecure bool
	}{
		{
			name:     "rtmp",
			endpoint: "rtmpconns",
			path:     "mystream",
			isSecure: false,
		},
		{
			name:     "rtmps",
			endpoint: "rtmpsconns",
			path:     "secure-stream",
			isSecure: true,
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			id := uuid.New()
			now := time.Now()

			rtmpServer := &testRTMPServer{
				conns: map[uuid.UUID]*defs.APIRTMPConn{
					id: {
						ID:            id,
						Created:       now,
						RemoteAddr:    "192.168.1.100:5000",
						State:         defs.APIRTMPConnStatePublish,
						Path:          ca.path,
						Query:         "key=value",
						BytesReceived: 999999,
						BytesSent:     888888,
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

			if ca.isSecure {
				api.RTMPSServer = rtmpServer
			} else {
				api.RTMPServer = rtmpServer
			}

			err := api.Initialize()
			require.NoError(t, err)
			defer api.Close()

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr}

			var out defs.APIRTMPConn
			httpRequest(t, hc, http.MethodGet, fmt.Sprintf("http://localhost:9997/v3/%s/get/%s", ca.endpoint, id), nil, &out)

			require.Equal(t, id, out.ID)
			require.Equal(t, "192.168.1.100:5000", out.RemoteAddr)
			require.Equal(t, defs.APIRTMPConnStatePublish, out.State)
			require.Equal(t, ca.path, out.Path)
			require.Equal(t, uint64(999999), out.BytesReceived)
		})
	}
}

func TestRTMPConnsKick(t *testing.T) {
	for _, ca := range []struct {
		name     string
		endpoint string
		path     string
		isSecure bool
	}{
		{
			name:     "rtmp",
			endpoint: "rtmpconns",
			path:     "mystream",
			isSecure: false,
		},
		{
			name:     "rtmps",
			endpoint: "rtmpsconns",
			path:     "secure-stream",
			isSecure: true,
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			id := uuid.New()
			now := time.Now()

			rtmpServer := &testRTMPServer{
				conns: map[uuid.UUID]*defs.APIRTMPConn{
					id: {
						ID:            id,
						Created:       now,
						RemoteAddr:    "192.168.1.100:5000",
						State:         defs.APIRTMPConnStatePublish,
						Path:          ca.path,
						Query:         "",
						BytesReceived: 1000,
						BytesSent:     2000,
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

			if ca.isSecure {
				api.RTMPSServer = rtmpServer
			} else {
				api.RTMPServer = rtmpServer
			}

			err := api.Initialize()
			require.NoError(t, err)
			defer api.Close()

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr}

			httpRequest(t, hc, http.MethodPost, fmt.Sprintf("http://localhost:9997/v3/%s/kick/%s", ca.endpoint, id), nil, nil)
		})
	}
}
