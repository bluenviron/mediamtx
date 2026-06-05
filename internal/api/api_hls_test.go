package api //nolint:revive

import (
	"fmt"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/servers/hls"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type testHLSServer struct {
	muxers   map[string]*defs.APIHLSMuxer
	sessions map[string]*defs.APIHLSSession
}

var testTime = time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

func (s *testHLSServer) APIMuxersList() (*defs.APIHLSMuxerList, error) {
	items := make([]defs.APIHLSMuxer, 0, len(s.muxers))
	for _, muxer := range s.muxers {
		items = append(items, *muxer)
	}
	return &defs.APIHLSMuxerList{Items: items}, nil
}

func (s *testHLSServer) APIMuxersGet(name string) (*defs.APIHLSMuxer, error) {
	muxer, ok := s.muxers[name]
	if !ok {
		return nil, hls.ErrMuxerNotFound
	}
	return muxer, nil
}

func (s *testHLSServer) APISessionsList() (*defs.APIHLSSessionList, error) {
	items := make([]defs.APIHLSSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		items = append(items, *session)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Created.Before(items[j].Created)
	})
	return &defs.APIHLSSessionList{Items: items}, nil
}

func (s *testHLSServer) APISessionsGet(id uuid.UUID) (*defs.APIHLSSession, error) {
	session, ok := s.sessions[id.String()]
	if !ok {
		return nil, hls.ErrSessionNotFound
	}
	return session, nil
}

func (s *testHLSServer) APISessionsKick(id uuid.UUID) error {
	_, ok := s.sessions[id.String()]
	if !ok {
		return hls.ErrSessionNotFound
	}
	delete(s.sessions, id.String())
	return nil
}

func TestHLSMuxersList(t *testing.T) {
	now := testTime
	hlsServer := &testHLSServer{
		muxers: map[string]*defs.APIHLSMuxer{
			"test1": {
				Path:                    "test1",
				Created:                 now,
				LastRequest:             now.Add(5 * time.Second),
				OutboundBytes:           1234,
				OutboundFramesDiscarded: 10,
				BytesSent:               1234,
			},
			"test2": {
				Path:                    "test2",
				Created:                 now.Add(time.Minute),
				LastRequest:             now.Add(time.Minute + 10*time.Second),
				OutboundBytes:           5678,
				OutboundFramesDiscarded: 20,
				BytesSent:               5678,
			},
		},
	}

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		HLSServer:    hlsServer,
		Parent:       &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var out defs.APIHLSMuxerList
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/hlsmuxers/list", nil, &out)

	require.Equal(t, 2, out.ItemCount)
	require.Equal(t, 1, out.PageCount)
	require.Len(t, out.Items, 2)
}

func TestHLSMuxersGet(t *testing.T) {
	now := testTime
	hlsServer := &testHLSServer{
		muxers: map[string]*defs.APIHLSMuxer{
			"mypath": {
				Path:                    "mypath",
				Created:                 now,
				LastRequest:             now.Add(5 * time.Second),
				OutboundBytes:           9999,
				OutboundFramesDiscarded: 12,
				BytesSent:               9999,
			},
		},
	}

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		HLSServer:    hlsServer,
		Parent:       &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var out defs.APIHLSMuxer
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/hlsmuxers/get/mypath", nil, &out)

	require.Equal(t, "mypath", out.Path)
	require.Equal(t, uint64(9999), out.OutboundBytes)
	require.Equal(t, uint64(12), out.OutboundFramesDiscarded)
	require.Equal(t, uint64(9999), out.BytesSent)
}

func TestHLSSessionsList(t *testing.T) {
	now := testTime
	hlsServer := &testHLSServer{
		sessions: map[string]*defs.APIHLSSession{
			"session1": {
				ID:            uuid.MustParse("18294761-f9d1-4ea9-9a35-fe265b62eb41"),
				Created:       now,
				RemoteAddr:    "192.168.1.1:5000",
				Path:          "stream1",
				Query:         "key=val1",
				User:          "user1",
				OutboundBytes: 111,
			},
			"session2": {
				ID:            uuid.MustParse("18294761-f9d1-4ea9-9a35-fe265b62eb42"),
				Created:       now.Add(time.Minute),
				RemoteAddr:    "192.168.1.2:5001",
				Path:          "stream2",
				Query:         "key=val2",
				User:          "user2",
				OutboundBytes: 222,
			},
		},
	}

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		HLSServer:    hlsServer,
		Parent:       &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var out defs.APIHLSSessionList
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/hlssessions/list", nil, &out)

	require.Equal(t, 2, out.ItemCount)
	require.Equal(t, 1, out.PageCount)
	require.Len(t, out.Items, 2)
	require.Equal(t, []defs.APIHLSSession{
		{
			ID:            uuid.MustParse("18294761-f9d1-4ea9-9a35-fe265b62eb41"),
			Created:       now,
			RemoteAddr:    "192.168.1.1:5000",
			Path:          "stream1",
			Query:         "key=val1",
			User:          "user1",
			OutboundBytes: 111,
		},
		{
			ID:            uuid.MustParse("18294761-f9d1-4ea9-9a35-fe265b62eb42"),
			Created:       now.Add(time.Minute),
			RemoteAddr:    "192.168.1.2:5001",
			Path:          "stream2",
			Query:         "key=val2",
			User:          "user2",
			OutboundBytes: 222,
		},
	}, out.Items)
}

func TestHLSSessionsGet(t *testing.T) {
	now := testTime
	hlsServer := &testHLSServer{
		sessions: map[string]*defs.APIHLSSession{
			"18294761-f9d1-4ea9-9a35-fe265b62eb41": {
				ID:            uuid.MustParse("18294761-f9d1-4ea9-9a35-fe265b62eb41"),
				Created:       now,
				RemoteAddr:    "192.168.1.100:5000",
				Path:          "mystream",
				Query:         "key=val",
				User:          "myuser",
				OutboundBytes: 345,
			},
		},
	}

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		HLSServer:    hlsServer,
		Parent:       &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	sessionID := "18294761-f9d1-4ea9-9a35-fe265b62eb41"

	var out defs.APIHLSSession
	httpRequest(t, hc, http.MethodGet, fmt.Sprintf("http://localhost:9997/v3/hlssessions/get/%s", sessionID), nil, &out)

	require.Equal(t, uuid.MustParse(sessionID), out.ID)
	require.Equal(t, "192.168.1.100:5000", out.RemoteAddr)
	require.Equal(t, "mystream", out.Path)
	require.Equal(t, "key=val", out.Query)
	require.Equal(t, "myuser", out.User)
	require.Equal(t, uint64(345), out.OutboundBytes)
}

func TestHLSSessionsKick(t *testing.T) {
	now := testTime
	sessionID := uuid.MustParse("18294761-f9d1-4ea9-9a35-fe265b62eb41")
	hlsServer := &testHLSServer{
		sessions: map[string]*defs.APIHLSSession{
			sessionID.String(): {
				ID:            sessionID,
				Created:       now,
				RemoteAddr:    "192.168.1.100:5000",
				Path:          "mystream",
				OutboundBytes: 345,
			},
		},
	}

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		HLSServer:    hlsServer,
		Parent:       &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	httpRequest(t, hc, http.MethodPost, fmt.Sprintf("http://localhost:9997/v3/hlssessions/kick/%s", sessionID), nil, nil)

	_, ok := hlsServer.sessions[sessionID.String()]
	require.False(t, ok)
}

func TestHLSSessionsKickNotFound(t *testing.T) {
	hlsServer := &testHLSServer{}

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		HLSServer:    hlsServer,
		Parent:       &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	req, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("http://localhost:9997/v3/hlssessions/kick/%s", uuid.New()), nil)
	require.NoError(t, err)

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNotFound, res.StatusCode)
	checkError(t, res.Body, "session not found")
}
