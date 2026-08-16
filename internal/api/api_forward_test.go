package api //nolint:revive

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/forward"
	"github.com/bluenviron/mediamtx/internal/test"
)

type testForwardPathManager struct {
	items map[uuid.UUID]*defs.APIForwardDest
}

func (*testForwardPathManager) APIPathsList() (*defs.APIPathList, error) {
	return &defs.APIPathList{}, nil
}

func (*testForwardPathManager) APIPathsGet(string) (*defs.APIPath, error) {
	return &defs.APIPath{}, nil
}

func (m *testForwardPathManager) APIForwardDestList(path string) (*defs.APIForwardDestList, error) {
	if path != "my/nested/stream" {
		return nil, conf.ErrPathNotFound
	}

	items := make([]defs.APIForwardDest, 0, len(m.items))
	for _, item := range m.items {
		items = append(items, *item)
	}

	return &defs.APIForwardDestList{Items: items}, nil
}

func (m *testForwardPathManager) APIForwardDestGet(path string, id uuid.UUID) (*defs.APIForwardDest, error) {
	if path != "my/nested/stream" {
		return nil, conf.ErrPathNotFound
	}

	item, ok := m.items[id]
	if !ok {
		return nil, forward.ErrDestNotFound
	}

	return item, nil
}

func TestForward(t *testing.T) {
	rtmpID := uuid.New()
	whipID := uuid.New()
	pathManager := &testForwardPathManager{
		items: map[uuid.UUID]*defs.APIForwardDest{
			rtmpID: {
				ID:            rtmpID,
				Pos:           1,
				Created:       time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC),
				Conf:          conf.ForwardDest{Dest: "rtmp://localhost/live/stream"},
				Protocol:      defs.APIForwardDestProtocolRTMP,
				State:         defs.APIForwardDestStateError,
				LastError:     "connection refused",
				OutboundBytes: 123,
			},
			whipID: {
				ID:            whipID,
				Pos:           2,
				Created:       time.Date(2026, 6, 18, 9, 1, 0, 0, time.UTC),
				Conf:          conf.ForwardDest{Dest: "whip://localhost/live/stream/whip", WhipBearerToken: "mytoken"},
				Protocol:      defs.APIForwardDestProtocolWHIP,
				State:         defs.APIForwardDestStateForwarding,
				OutboundBytes: 456,
			},
		},
	}

	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		PathManager:  pathManager,
		Parent:       &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var list defs.APIForwardDestList
	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9997/v3/paths/forward/list?path=my%2Fnested%2Fstream", nil, &list)
	require.Equal(t, 2, list.ItemCount)
	require.Equal(t, 1, list.PageCount)
	require.Len(t, list.Items, 2)

	require.ElementsMatch(t, []defs.APIForwardDest{
		{
			ID:            rtmpID,
			Pos:           1,
			Created:       time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC),
			Conf:          conf.ForwardDest{Dest: "rtmp://localhost/live/stream"},
			Protocol:      defs.APIForwardDestProtocolRTMP,
			State:         defs.APIForwardDestStateError,
			LastError:     "connection refused",
			OutboundBytes: 123,
		},
		{
			ID:            whipID,
			Pos:           2,
			Created:       time.Date(2026, 6, 18, 9, 1, 0, 0, time.UTC),
			Conf:          conf.ForwardDest{Dest: "whip://localhost/live/stream/whip", WhipBearerToken: "mytoken"},
			Protocol:      defs.APIForwardDestProtocolWHIP,
			State:         defs.APIForwardDestStateForwarding,
			OutboundBytes: 456,
		},
	}, list.Items)

	var item defs.APIForwardDest
	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9997/v3/paths/forward/get?path=my%2Fnested%2Fstream&id="+whipID.String(), nil, &item)
	require.Equal(t, "whip://localhost/live/stream/whip", item.Conf.Dest)
	require.Equal(t, "mytoken", item.Conf.WhipBearerToken)
	require.Equal(t, defs.APIForwardDestProtocolWHIP, item.Protocol)
	require.Equal(t, defs.APIForwardDestStateForwarding, item.State)
	require.Equal(t, uint64(456), item.OutboundBytes)
}
