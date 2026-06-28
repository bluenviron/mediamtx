package api //nolint:revive

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/push"
	"github.com/bluenviron/mediamtx/internal/test"
)

type testPushTargetPathManager struct {
	items map[uuid.UUID]*defs.APIPushTarget
}

func (*testPushTargetPathManager) APIPathsList() (*defs.APIPathList, error) {
	return &defs.APIPathList{}, nil
}

func (*testPushTargetPathManager) APIPathsGet(string) (*defs.APIPath, error) {
	return &defs.APIPath{}, nil
}

func (m *testPushTargetPathManager) APIPushTargetsList(string) (*defs.APIPushTargetList, error) {
	items := make([]defs.APIPushTarget, 0, len(m.items))
	for _, item := range m.items {
		items = append(items, *item)
	}

	return &defs.APIPushTargetList{Items: items}, nil
}

func (m *testPushTargetPathManager) APIPushTargetsGet(_ string, id uuid.UUID) (*defs.APIPushTarget, error) {
	item, ok := m.items[id]
	if !ok {
		return nil, push.ErrTargetNotFound
	}

	return item, nil
}

func (m *testPushTargetPathManager) APIPushTargetsAdd(
	_ string,
	req defs.APIPushTargetAdd,
) (*defs.APIPushTarget, error) {
	id := uuid.New()
	item := &defs.APIPushTarget{
		ID:       id,
		Created:  time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC),
		URL:      req.URL,
		Protocol: defs.APIPushTargetProtocolSRT,
		Source:   defs.APIPushTargetSourceAPI,
		State:    defs.APIPushTargetStateConnecting,
	}
	m.items[id] = item

	return item, nil
}

func (m *testPushTargetPathManager) APIPushTargetsRemove(_ string, id uuid.UUID) error {
	if _, ok := m.items[id]; !ok {
		return push.ErrTargetNotFound
	}

	delete(m.items, id)
	return nil
}

func TestPushTargets(t *testing.T) {
	id := uuid.New()
	pathManager := &testPushTargetPathManager{
		items: map[uuid.UUID]*defs.APIPushTarget{
			id: {
				ID:            id,
				Created:       time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC),
				URL:           "rtmp://localhost/live/stream",
				Protocol:      defs.APIPushTargetProtocolRTMP,
				Source:        defs.APIPushTargetSourceConfig,
				State:         defs.APIPushTargetStateError,
				LastError:     "connection refused",
				OutboundBytes: 123,
				BytesSent:     123,
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

	var list defs.APIPushTargetList
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/pushtargets/list/mystream", nil, &list)
	require.Equal(t, 1, list.ItemCount)
	require.Equal(t, 1, list.PageCount)
	require.Equal(t, id, list.Items[0].ID)

	var item defs.APIPushTarget
	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9997/v3/paths/pushtargets/get/"+id.String()+"/mystream", nil, &item)
	require.Equal(t, "rtmp://localhost/live/stream", item.URL)
	require.Equal(t, defs.APIPushTargetProtocolRTMP, item.Protocol)
	require.Equal(t, defs.APIPushTargetStateError, item.State)
	require.Equal(t, "connection refused", item.LastError)
	require.Equal(t, uint64(123), item.OutboundBytes)
	require.Equal(t, uint64(123), item.BytesSent)

	var added defs.APIPushTarget
	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/paths/pushtargets/add/mystream",
		defs.APIPushTargetAdd{URL: "srt://localhost:8890?streamid=publish:mystream"}, &added)
	require.Equal(t, defs.APIPushTargetSourceAPI, added.Source)
	require.Equal(t, "srt://localhost:8890?streamid=publish:mystream", added.URL)
	require.Equal(t, defs.APIPushTargetProtocolSRT, added.Protocol)

	httpRequest(t, hc, http.MethodDelete,
		"http://localhost:9997/v3/paths/pushtargets/remove/"+added.ID.String()+"/mystream", nil, nil)
	_, ok := pathManager.items[added.ID]
	require.False(t, ok)
}
