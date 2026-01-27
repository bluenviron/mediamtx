package api //nolint:revive

import (
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/servers/hls"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

type testHLSServer struct {
	muxers map[string]*defs.APIHLSMuxer
}

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

func TestHLSMuxersList(t *testing.T) {
	now := time.Now()
	hlsServer := &testHLSServer{
		muxers: map[string]*defs.APIHLSMuxer{
			"test1": {
				Path:        "test1",
				Created:     now,
				LastRequest: now.Add(5 * time.Second),
				BytesSent:   1234,
			},
			"test2": {
				Path:        "test2",
				Created:     now.Add(time.Minute),
				LastRequest: now.Add(time.Minute + 10*time.Second),
				BytesSent:   5678,
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
	now := time.Now()
	hlsServer := &testHLSServer{
		muxers: map[string]*defs.APIHLSMuxer{
			"mypath": {
				Path:        "mypath",
				Created:     now,
				LastRequest: now.Add(5 * time.Second),
				BytesSent:   9999,
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
	require.Equal(t, uint64(9999), out.BytesSent)
}
