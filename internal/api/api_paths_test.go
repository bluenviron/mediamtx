package api //nolint:revive

import (
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

type testPathManager struct {
	paths map[string]*defs.APIPath
}

func (m *testPathManager) APIPathsList() (*defs.APIPathList, error) {
	items := make([]defs.APIPath, 0, len(m.paths))
	for _, path := range m.paths {
		items = append(items, *path)
	}
	return &defs.APIPathList{Items: items}, nil
}

func (m *testPathManager) APIPathsGet(name string) (*defs.APIPath, error) {
	path, ok := m.paths[name]
	if !ok {
		return nil, conf.ErrPathNotFound
	}
	return path, nil
}

func TestPathsList(t *testing.T) {
	now := time.Now()
	pathManager := &testPathManager{
		paths: map[string]*defs.APIPath{
			"test1": {
				Name:          "test1",
				ConfName:      "test1",
				Source:        &defs.APIPathSource{Type: "publisher", ID: "pub1"},
				Ready:         true,
				ReadyTime:     &now,
				Tracks:        []string{"H264", "Opus"},
				BytesReceived: 1000,
				BytesSent:     2000,
				Readers: []defs.APIPathReader{
					{Type: "reader", ID: "reader1"},
				},
			},
			"test2": {
				Name:          "test2",
				ConfName:      "test2",
				Ready:         false,
				Tracks:        []string{},
				BytesReceived: 500,
				BytesSent:     100,
				Readers:       []defs.APIPathReader{},
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

	var out defs.APIPathList
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/list", nil, &out)

	require.Equal(t, 2, out.ItemCount)
	require.Equal(t, 1, out.PageCount)
	require.Len(t, out.Items, 2)
}

func TestPathsGet(t *testing.T) {
	now := time.Now()
	pathManager := &testPathManager{
		paths: map[string]*defs.APIPath{
			"mystream": {
				Name:          "mystream",
				ConfName:      "mystream",
				Source:        &defs.APIPathSource{Type: "rtspSession", ID: "session123"},
				Ready:         true,
				ReadyTime:     &now,
				Tracks:        []string{"H264", "Opus"},
				BytesReceived: 123456,
				BytesSent:     789012,
				Readers: []defs.APIPathReader{
					{Type: "hlsMuxer", ID: "muxer1"},
					{Type: "webRTCSession", ID: "session456"},
				},
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

	var out defs.APIPath
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/get/mystream", nil, &out)

	require.Equal(t, "mystream", out.Name)
	require.Equal(t, "mystream", out.ConfName)
	require.True(t, out.Ready)
	require.NotNil(t, out.Source)
	require.Equal(t, "rtspSession", out.Source.Type)
	require.Len(t, out.Tracks, 2)
	require.Len(t, out.Readers, 2)
	require.Equal(t, uint64(123456), out.BytesReceived)
	require.Equal(t, uint64(789012), out.BytesSent)
}
