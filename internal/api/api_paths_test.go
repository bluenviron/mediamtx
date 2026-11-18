package api //nolint:revive

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/formatlabel"
	"github.com/bluenviron/mediamtx/internal/test"
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

func (*testPathManager) APIForwardDestsList(string) (*defs.APIForwardDestList, error) {
	return &defs.APIForwardDestList{}, nil
}

func (*testPathManager) APIReadersList(path string) (*defs.APIPathReaderList, error) {
	if path != "mypath" {
		return nil, conf.ErrPathNotFound
	}

	return &defs.APIPathReaderList{Items: []defs.APIPathReader{
		{Type: defs.APIPathReaderTypeRTSPSession, ID: "id1"},
		{Type: defs.APIPathReaderTypeHLSSession, ID: "id2"},
	}}, nil
}

func (*testPathManager) APIReadersGet(path string, id string) (*defs.APIPathReader, error) {
	if path != "mypath" {
		return nil, conf.ErrPathNotFound
	}

	if id != "id1" {
		return nil, defs.ErrPathReaderNotFound
	}

	return &defs.APIPathReader{Type: defs.APIPathReaderTypeRTSPSession, ID: "id1"}, nil
}

func (*testPathManager) APIForwardDestsGet(string, uuid.UUID) (*defs.APIForwardDest, error) {
	return nil, conf.ErrPathNotFound
}

func (*testPathManager) APIStaticSourcesGet(string) (*defs.APIStaticSource, error) {
	return nil, conf.ErrPathNotFound
}

func TestPathsList(t *testing.T) {
	now := time.Now()
	pathManager := &testPathManager{
		paths: map[string]*defs.APIPath{
			"test1": {
				Name:                 "test1",
				ConfName:             "test1",
				Source:               &defs.APIPathSource{Type: defs.APIPathSourceTypeRTSPSession, ID: "pub1"},
				Ready:                true,
				ReadyTime:            &now,
				Tracks:               []defs.APIPathTrackCodec{formatlabel.H264, formatlabel.Opus},
				InboundBytes:         1000,
				OutboundBytes:        2000,
				InboundFramesInError: 3,
				BytesReceived:        1000,
				BytesSent:            2000,
				Readers: []defs.APIPathReader{
					{Type: defs.APIPathReaderTypeRTSPSession, ID: "reader1"},
				},
			},
			"test2": {
				Name:                 "test2",
				ConfName:             "test2",
				Ready:                false,
				Tracks:               []defs.APIPathTrackCodec{},
				InboundBytes:         500,
				OutboundBytes:        100,
				InboundFramesInError: 1,
				BytesReceived:        500,
				BytesSent:            100,
				Readers:              []defs.APIPathReader{},
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
				Name:                 "mystream",
				ConfName:             "mystream",
				Source:               &defs.APIPathSource{Type: defs.APIPathSourceTypeRTSPSession, ID: "session123"},
				Ready:                true,
				ReadyTime:            &now,
				Tracks:               []defs.APIPathTrackCodec{formatlabel.H264, formatlabel.Opus},
				InboundBytes:         123456,
				OutboundBytes:        789012,
				InboundFramesInError: 12,
				BytesReceived:        123456,
				BytesSent:            789012,
				Readers: []defs.APIPathReader{
					{Type: defs.APIPathReaderTypeHLSSession, ID: "session6123"},
					{Type: defs.APIPathReaderTypeWebRTCSession, ID: "session456"},
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
	require.Equal(t, defs.APIPathSourceTypeRTSPSession, out.Source.Type)
	require.Len(t, out.Tracks, 2)
	require.Len(t, out.Readers, 2)
	require.Equal(t, uint64(123456), out.InboundBytes)
	require.Equal(t, uint64(789012), out.OutboundBytes)
	require.Equal(t, uint64(12), out.InboundFramesInError)
	require.Equal(t, uint64(123456), out.BytesReceived)
	require.Equal(t, uint64(789012), out.BytesSent)
}

func TestPathReaders(t *testing.T) {
	api := API{
		Address:      "localhost:9997",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		AuthManager:  test.NilAuthManager,
		PathManager:  &testPathManager{},
		Parent:       &testParent{},
	}
	err := api.Initialize()
	require.NoError(t, err)
	defer api.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var list defs.APIPathReaderList
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/readers/list?path=mypath", nil, &list)
	require.Equal(t, 2, list.ItemCount)
	require.Equal(t, 1, list.PageCount)
	require.Len(t, list.Items, 2)

	var item defs.APIPathReader
	httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/readers/get?path=mypath&id=id1", nil, &item)
	require.Equal(t, defs.APIPathReader{Type: defs.APIPathReaderTypeRTSPSession, ID: "id1"}, item)

	for _, ca := range []struct {
		url    string
		status int
	}{
		{"list", http.StatusBadRequest},
		{"list?path=other", http.StatusNotFound},
		{"get?path=mypath", http.StatusBadRequest},
		{"get?path=other&id=id1", http.StatusNotFound},
		{"get?path=mypath&id=nope", http.StatusNotFound},
	} {
		res, err2 := hc.Get("http://localhost:9997/v3/paths/readers/" + ca.url)
		require.NoError(t, err2)
		res.Body.Close()
		require.Equal(t, ca.status, res.StatusCode, ca.url)
	}
}
