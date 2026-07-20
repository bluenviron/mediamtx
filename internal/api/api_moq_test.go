package api //nolint:revive

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/servers/moq"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type testMoQServer struct {
	sessions map[uuid.UUID]*defs.APIMoQSession
}

func (s *testMoQServer) APISessionsList() (*defs.APIMoQSessionList, error) {
	items := make([]defs.APIMoQSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		items = append(items, *session)
	}
	return &defs.APIMoQSessionList{Items: items}, nil
}

func (s *testMoQServer) APISessionsGet(id uuid.UUID) (*defs.APIMoQSession, error) {
	session, ok := s.sessions[id]
	if !ok {
		return nil, moq.ErrSessionNotFound
	}
	return session, nil
}

func (s *testMoQServer) APISessionsKick(id uuid.UUID) error {
	_, ok := s.sessions[id]
	if !ok {
		return moq.ErrSessionNotFound
	}
	return nil
}

func TestMoQSessionsList(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	now := time.Now()

	moqServer := &testMoQServer{
		sessions: map[uuid.UUID]*defs.APIMoQSession{
			id1: {
				ID:            id1,
				Created:       now,
				RemoteAddr:    "192.168.1.1:5000",
				State:         defs.APIMoQSessionStatePublish,
				Path:          "stream1",
				Query:         "token=abc",
				Version:       defs.APIMoQVersionDraft19,
				InboundBytes:  1000,
				OutboundBytes: 2000,
			},
			id2: {
				ID:            id2,
				Created:       now.Add(time.Minute),
				RemoteAddr:    "192.168.1.2:5001",
				State:         defs.APIMoQSessionStateRead,
				Path:          "stream2",
				Query:         "",
				Version:       defs.APIMoQVersionDraft18,
				InboundBytes:  500,
				OutboundBytes: 1500,
			},
		},
	}

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		MoQServer:    moqServer,
		Parent:       &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var out defs.APIMoQSessionList
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/moqsessions/list", nil, &out)

	require.Equal(t, 2, out.ItemCount)
	require.Equal(t, 1, out.PageCount)
	require.Len(t, out.Items, 2)
}

func TestMoQSessionsGet(t *testing.T) {
	id := uuid.New()
	now := time.Now()

	moqServer := &testMoQServer{
		sessions: map[uuid.UUID]*defs.APIMoQSession{
			id: {
				ID:            id,
				Created:       now,
				RemoteAddr:    "192.168.1.100:5000",
				State:         defs.APIMoQSessionStatePublish,
				Path:          "mystream",
				Query:         "key=value",
				Version:       defs.APIMoQVersionDraft19,
				InboundBytes:  999999,
				OutboundBytes: 888888,
			},
		},
	}

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		MoQServer:    moqServer,
		Parent:       &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var out defs.APIMoQSession
	httpRequest(t, hc, http.MethodGet, fmt.Sprintf("http://localhost:9997/v3/moqsessions/get/%s", id), nil, &out)

	require.Equal(t, id, out.ID)
	require.Equal(t, "192.168.1.100:5000", out.RemoteAddr)
	require.Equal(t, defs.APIMoQSessionStatePublish, out.State)
	require.Equal(t, "mystream", out.Path)
	require.Equal(t, uint64(999999), out.InboundBytes)
	require.Equal(t, uint64(888888), out.OutboundBytes)
}

func TestMoQSessionsKick(t *testing.T) {
	id := uuid.New()
	now := time.Now()

	moqServer := &testMoQServer{
		sessions: map[uuid.UUID]*defs.APIMoQSession{
			id: {
				ID:            id,
				Created:       now,
				RemoteAddr:    "192.168.1.100:5000",
				State:         defs.APIMoQSessionStatePublish,
				Path:          "mystream",
				Query:         "",
				Version:       defs.APIMoQVersionDraft19,
				InboundBytes:  1000,
				OutboundBytes: 2000,
			},
		},
	}

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		MoQServer:    moqServer,
		Parent:       &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	httpRequest(t, hc, http.MethodPost, fmt.Sprintf("http://localhost:9997/v3/moqsessions/kick/%s", id), nil, nil)
}
