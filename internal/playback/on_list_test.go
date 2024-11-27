package playback

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestOnList(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-playback")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.Mkdir(filepath.Join(dir, "mypath"), 0o755)
	require.NoError(t, err)

	writeSegment1(t, filepath.Join(dir, "mypath", "2008-11-07_11-22-00-500000.mp4"))
	writeSegment2(t, filepath.Join(dir, "mypath", "2008-11-07_11-23-02-500000.mp4"))
	writeSegment2(t, filepath.Join(dir, "mypath", "2009-11-07_11-23-02-500000.mp4"))

	s := &Server{
		Address:     "127.0.0.1:9996",
		ReadTimeout: conf.StringDuration(10 * time.Second),
		PathConfs: map[string]*conf.Path{
			"mypath": {
				Name:       "mypath",
				RecordPath: filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
			},
		},
		AuthManager: test.NilAuthManager,
		Parent:      test.NilLogger,
	}
	err = s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	u, err := url.Parse("http://myuser:mypass@localhost:9996/list")
	require.NoError(t, err)

	v := url.Values{}
	v.Set("path", "mypath")
	u.RawQuery = v.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)

	var out interface{}
	err = json.NewDecoder(res.Body).Decode(&out)
	require.NoError(t, err)

	require.Equal(t, []interface{}{
		map[string]interface{}{
			"duration": float64(65),
			"start":    time.Date(2008, 11, 0o7, 11, 22, 0, 500000000, time.Local).Format(time.RFC3339Nano),
			"url": "http://localhost:9996/get?duration=65&path=mypath&start=" +
				url.QueryEscape(time.Date(2008, 11, 0o7, 11, 22, 0, 500000000, time.Local).Format(time.RFC3339Nano)),
		},
		map[string]interface{}{
			"duration": float64(3),
			"start":    time.Date(2009, 11, 0o7, 11, 23, 2, 500000000, time.Local).Format(time.RFC3339Nano),
			"url": "http://localhost:9996/get?duration=3&path=mypath&start=" +
				url.QueryEscape(time.Date(2009, 11, 0o7, 11, 23, 2, 500000000, time.Local).Format(time.RFC3339Nano)),
		},
	}, out)
}

func TestOnListDifferentInit(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-playback")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.Mkdir(filepath.Join(dir, "mypath"), 0o755)
	require.NoError(t, err)

	writeSegment1(t, filepath.Join(dir, "mypath", "2008-11-07_11-22-00-500000.mp4"))
	writeSegment3(t, filepath.Join(dir, "mypath", "2008-11-07_11-23-02-500000.mp4"))

	s := &Server{
		Address:     "127.0.0.1:9996",
		ReadTimeout: conf.StringDuration(10 * time.Second),
		PathConfs: map[string]*conf.Path{
			"mypath": {
				Name:       "mypath",
				RecordPath: filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
			},
		},
		AuthManager: test.NilAuthManager,
		Parent:      test.NilLogger,
	}
	err = s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	u, err := url.Parse("http://myuser:mypass@localhost:9996/list")
	require.NoError(t, err)

	v := url.Values{}
	v.Set("path", "mypath")
	u.RawQuery = v.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)

	var out interface{}
	err = json.NewDecoder(res.Body).Decode(&out)
	require.NoError(t, err)

	require.Equal(t, []interface{}{
		map[string]interface{}{
			"duration": float64(62),
			"start":    time.Date(2008, 11, 0o7, 11, 22, 0, 500000000, time.Local).Format(time.RFC3339Nano),
			"url": "http://localhost:9996/get?duration=62&path=mypath&start=" +
				url.QueryEscape(time.Date(2008, 11, 0o7, 11, 22, 0, 500000000, time.Local).Format(time.RFC3339Nano)),
		},
		map[string]interface{}{
			"duration": float64(1),
			"start":    time.Date(2008, 11, 0o7, 11, 23, 2, 500000000, time.Local).Format(time.RFC3339Nano),
			"url": "http://localhost:9996/get?duration=1&path=mypath&start=" +
				url.QueryEscape(time.Date(2008, 11, 0o7, 11, 23, 2, 500000000, time.Local).Format(time.RFC3339Nano)),
		},
	}, out)
}
