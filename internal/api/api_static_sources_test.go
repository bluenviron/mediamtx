package api //nolint:revive

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/staticsources"
	"github.com/bluenviron/mediamtx/internal/test"
)

type testStaticSourcesPathManager struct {
	item *defs.APIStaticSource
}

func (*testStaticSourcesPathManager) APIPathsList() (*defs.APIPathList, error) {
	return &defs.APIPathList{}, nil
}

func (*testStaticSourcesPathManager) APIPathsGet(string) (*defs.APIPath, error) {
	return &defs.APIPath{}, nil
}

func (*testStaticSourcesPathManager) APIForwardDestsList(string) (*defs.APIForwardDestList, error) {
	return &defs.APIForwardDestList{}, nil
}

func (*testStaticSourcesPathManager) APIForwardDestsGet(string, uuid.UUID) (*defs.APIForwardDest, error) {
	return nil, conf.ErrPathNotFound
}

func (m *testStaticSourcesPathManager) APIStaticSourcesGet(path string) (*defs.APIStaticSource, error) {
	switch path {
	case "my/nested/stream":
		return m.item, nil

	case "no_source":
		return nil, staticsources.ErrNoStaticSource
	}

	return nil, conf.ErrPathNotFound
}

func TestStaticSourcesGet(t *testing.T) {
	pathManager := &testStaticSourcesPathManager{
		item: &defs.APIStaticSource{
			Type:      defs.APIPathSourceTypeRTSPSource,
			State:     defs.APIStaticSourceStateError,
			LastError: "connection refused",
			Created:   time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC),
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

	var item defs.APIStaticSource
	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9997/v3/paths/static-sources/get/my/nested/stream", nil, &item)
	require.Equal(t, *pathManager.item, item)

	req, err := http.NewRequest(http.MethodGet,
		"http://localhost:9997/v3/paths/static-sources/get/no_source", nil)
	require.NoError(t, err)

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}
