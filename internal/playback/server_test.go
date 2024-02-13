package playback

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4/seekablebuffer"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func writeSegment1(t *testing.T, fpath string) {
	init := fmp4.Init{
		Tracks: []*fmp4.InitTrack{{
			ID:        1,
			TimeScale: 90000,
			Codec: &fmp4.CodecH264{
				SPS: test.FormatH264.SPS,
				PPS: test.FormatH264.PPS,
			},
		}},
	}

	var buf1 seekablebuffer.Buffer
	err := init.Marshal(&buf1)
	require.NoError(t, err)

	var buf2 seekablebuffer.Buffer
	parts := fmp4.Parts{
		{
			SequenceNumber: 1,
			Tracks: []*fmp4.PartTrack{{
				ID:       1,
				BaseTime: 0,
				Samples:  []*fmp4.PartSample{},
			}},
		},
		{
			SequenceNumber: 1,
			Tracks: []*fmp4.PartTrack{{
				ID:       1,
				BaseTime: 30 * 90000,
				Samples: []*fmp4.PartSample{
					{
						Duration:        30 * 90000,
						IsNonSyncSample: false,
						Payload:         []byte{1, 2},
					},
					{
						Duration:        1 * 90000,
						IsNonSyncSample: false,
						Payload:         []byte{3, 4},
					},
					{
						Duration:        1 * 90000,
						IsNonSyncSample: true,
						Payload:         []byte{5, 6},
					},
				},
			}},
		},
	}
	err = parts.Marshal(&buf2)
	require.NoError(t, err)

	err = os.WriteFile(fpath, append(buf1.Bytes(), buf2.Bytes()...), 0o644)
	require.NoError(t, err)
}

func writeSegment2(t *testing.T, fpath string) {
	init := fmp4.Init{
		Tracks: []*fmp4.InitTrack{{
			ID:        1,
			TimeScale: 90000,
			Codec: &fmp4.CodecH264{
				SPS: test.FormatH264.SPS,
				PPS: test.FormatH264.PPS,
			},
		}},
	}

	var buf1 seekablebuffer.Buffer
	err := init.Marshal(&buf1)
	require.NoError(t, err)

	var buf2 seekablebuffer.Buffer
	parts := fmp4.Parts{
		{
			SequenceNumber: 1,
			Tracks: []*fmp4.PartTrack{{
				ID:       1,
				BaseTime: 0,
				Samples: []*fmp4.PartSample{
					{
						Duration:        1 * 90000,
						IsNonSyncSample: false,
						Payload:         []byte{7, 8},
					},
					{
						Duration:        1 * 90000,
						IsNonSyncSample: false,
						Payload:         []byte{9, 10},
					},
				},
			}},
		},
	}
	err = parts.Marshal(&buf2)
	require.NoError(t, err)

	err = os.WriteFile(fpath, append(buf1.Bytes(), buf2.Bytes()...), 0o644)
	require.NoError(t, err)
}

func TestServerGet(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-playback")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.Mkdir(filepath.Join(dir, "mypath"), 0o755)
	require.NoError(t, err)

	writeSegment1(t, filepath.Join(dir, "mypath", "2008-11-07_11-22-00-500000.mp4"))
	writeSegment2(t, filepath.Join(dir, "mypath", "2008-11-07_11-23-02-500000.mp4"))

	s := &Server{
		Address:     "127.0.0.1:9996",
		ReadTimeout: conf.StringDuration(10 * time.Second),
		PathConfs: map[string]*conf.Path{
			"mypath": {
				Playback:   true,
				RecordPath: filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
			},
		},
		Parent: &test.NilLogger{},
	}
	err = s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	v := url.Values{}
	v.Set("path", "mypath")
	v.Set("start", time.Date(2008, 11, 0o7, 11, 23, 1, 500000000, time.Local).Format(time.RFC3339Nano))
	v.Set("duration", "2")
	v.Set("format", "fmp4")

	u := &url.URL{
		Scheme:   "http",
		Host:     "localhost:9996",
		Path:     "/get",
		RawQuery: v.Encode(),
	}

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)

	buf, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	var parts fmp4.Parts
	err = parts.Unmarshal(buf)
	require.NoError(t, err)

	require.Equal(t, fmp4.Parts{
		{
			SequenceNumber: 0,
			Tracks: []*fmp4.PartTrack{
				{
					ID: 1,
					Samples: []*fmp4.PartSample{
						{
							Duration: 0,
							Payload:  []byte{3, 4},
						},
						{
							Duration:        90000,
							IsNonSyncSample: true,
							Payload:         []byte{5, 6},
						},
					},
				},
			},
		},
		{
			SequenceNumber: 0,
			Tracks: []*fmp4.PartTrack{
				{
					ID:       1,
					BaseTime: 90000,
					Samples: []*fmp4.PartSample{
						{
							Duration: 90000,
							Payload:  []byte{7, 8},
						},
					},
				},
			},
		},
	}, parts)
}

func TestServerList(t *testing.T) {
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
				Playback:   true,
				RecordPath: filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
			},
		},
		Parent: &test.NilLogger{},
	}
	err = s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	v := url.Values{}
	v.Set("path", "mypath")

	u := &url.URL{
		Scheme:   "http",
		Host:     "localhost:9996",
		Path:     "/list",
		RawQuery: v.Encode(),
	}

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
			"duration": float64(64),
			"start":    time.Date(2008, 11, 0o7, 11, 22, 0, 500000000, time.Local).Format(time.RFC3339Nano),
		},
		map[string]interface{}{
			"duration": float64(2),
			"start":    time.Date(2009, 11, 0o7, 11, 23, 2, 500000000, time.Local).Format(time.RFC3339Nano),
		},
	}, out)
}
