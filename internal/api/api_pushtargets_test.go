package api //nolint:revive

import (
	"net/http"
	"sort"
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
	targets map[string]map[uuid.UUID]*defs.APIPushTarget
}

func (*testPushTargetPathManager) APIPathsList() (*defs.APIPathList, error) {
	panic("unused")
}

func (*testPushTargetPathManager) APIPathsGet(string) (*defs.APIPath, error) {
	panic("unused")
}

func (m *testPushTargetPathManager) APIPushTargetsList(pathName string) (*defs.APIPushTargetList, error) {
	items, ok := m.targets[pathName]
	if !ok {
		return nil, conf.ErrPathNotFound
	}

	out := make([]*defs.APIPushTarget, 0, len(items))
	for _, item := range items {
		copied := *item
		out = append(out, &copied)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID.String() < out[j].ID.String()
	})

	return &defs.APIPushTargetList{Items: out}, nil
}

func (m *testPushTargetPathManager) APIPushTargetsGet(pathName string, id uuid.UUID) (*defs.APIPushTarget, error) {
	items, ok := m.targets[pathName]
	if !ok {
		return nil, conf.ErrPathNotFound
	}

	item, ok := items[id]
	if !ok {
		return nil, push.ErrTargetNotFound
	}

	copied := *item
	return &copied, nil
}

func (m *testPushTargetPathManager) APIPushTargetsAdd(pathName string, req defs.APIPushTargetAdd) (*defs.APIPushTarget, error) {
	items, ok := m.targets[pathName]
	if !ok {
		return nil, conf.ErrPathNotFound
	}

	item := &defs.APIPushTarget{
		ID:        uuid.New(),
		Created:   time.Now(),
		URL:       req.URL,
		State:     defs.APIPushTargetStateIdle,
		BytesSent: 0,
	}
	items[item.ID] = item

	copied := *item
	return &copied, nil
}

func (m *testPushTargetPathManager) APIPushTargetsRemove(pathName string, id uuid.UUID) error {
	items, ok := m.targets[pathName]
	if !ok {
		return conf.ErrPathNotFound
	}

	if _, ok := items[id]; !ok {
		return push.ErrTargetNotFound
	}

	delete(items, id)
	return nil
}

func TestPushTargetsLifecycle(t *testing.T) {
	pathManager := &testPushTargetPathManager{
		targets: map[string]map[uuid.UUID]*defs.APIPushTarget{
			"folder/stream": {},
		},
	}

	api := API{
		Address:      "localhost:9996",
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

	var added defs.APIPushTarget
	httpRequest(t, hc, http.MethodPost,
		"http://localhost:9996/v3/paths/pushtargets/add/folder/stream",
		defs.APIPushTargetAdd{URL: "rtmp://example.com/live/test"},
		&added)

	require.Equal(t, "rtmp://example.com/live/test", added.URL)
	require.NotEqual(t, uuid.Nil, added.ID)

	var listed defs.APIPushTargetList
	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9996/v3/paths/pushtargets/list/folder/stream",
		nil,
		&listed)

	require.Equal(t, 1, listed.ItemCount)
	require.Equal(t, 1, listed.PageCount)
	require.Len(t, listed.Items, 1)
	require.Equal(t, added.ID, listed.Items[0].ID)

	var got defs.APIPushTarget
	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9996/v3/paths/pushtargets/get/folder/stream/"+added.ID.String(),
		nil,
		&got)

	require.Equal(t, added.ID, got.ID)
	require.Equal(t, added.URL, got.URL)

	httpRequest(t, hc, http.MethodDelete,
		"http://localhost:9996/v3/paths/pushtargets/remove/folder/stream/"+added.ID.String(),
		nil,
		nil)

	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9996/v3/paths/pushtargets/list/folder/stream",
		nil,
		&listed)

	require.Equal(t, 0, listed.ItemCount)
	require.Len(t, listed.Items, 0)
}

func TestPushTargetsGetNotFound(t *testing.T) {
	pathManager := &testPushTargetPathManager{
		targets: map[string]map[uuid.UUID]*defs.APIPushTarget{
			"folder/stream": {},
		},
	}

	api := API{
		Address:      "localhost:9996",
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

	res, err := hc.Get("http://localhost:9996/v3/paths/pushtargets/get/folder/stream/" + uuid.New().String())
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNotFound, res.StatusCode)
	checkError(t, res.Body, "push target not found")
}
