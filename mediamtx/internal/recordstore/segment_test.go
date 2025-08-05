package recordstore

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/stretchr/testify/require"
)

func TestFindAllPathsWithSegments(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-recordstore")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.Mkdir(filepath.Join(dir, "path1"), 0o755)
	require.NoError(t, err)

	err = os.Mkdir(filepath.Join(dir, "path2"), 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "path1", "2015-05-19_22-15-25-000427.mp4"), []byte{1}, 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "path2", "2015-07-19_22-15-25-000427.mp4"), []byte{1}, 0o644)
	require.NoError(t, err)

	paths := FindAllPathsWithSegments(map[string]*conf.Path{
		"~^.*$": {
			Name:         "~^.*$",
			Regexp:       regexp.MustCompile("^.*$"),
			RecordPath:   filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
			RecordFormat: conf.RecordFormatFMP4,
		},
		"path2": {
			Name:         "path2",
			RecordPath:   filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
			RecordFormat: conf.RecordFormatFMP4,
		},
	})
	require.Equal(t, []string{"path1", "path2"}, paths)
}

func TestFindAllPathsWithSegmentsInvalidPath(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-recordstore")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.WriteFile(filepath.Join(dir, "_2015-05-19_22-15-25-000427.mp4"), []byte{1}, 0o644)
	require.NoError(t, err)

	paths := FindAllPathsWithSegments(map[string]*conf.Path{
		"~^.*$": {
			Name:         "~^.*$",
			Regexp:       regexp.MustCompile("^.*$"),
			RecordPath:   filepath.Join(dir, "%path_%Y-%m-%d_%H-%M-%S-%f"),
			RecordFormat: conf.RecordFormatFMP4,
		},
	})
	require.Equal(t, []string{}, paths)
}

func TestFindSegments(t *testing.T) {
	for _, ca := range []string{
		"no filtering",
		"filtering",
		"start before first",
	} {
		t.Run(ca, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "mediamtx-recordstore")
			require.NoError(t, err)
			defer os.RemoveAll(dir)

			err = os.Mkdir(filepath.Join(dir, "path1"), 0o755)
			require.NoError(t, err)

			err = os.Mkdir(filepath.Join(dir, "path2"), 0o755)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(dir, "path1", "2015-05-19_22-15-25-000427.mp4"), []byte{1}, 0o644)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(dir, "path1", "2016-05-19_22-15-25-000427.mp4"), []byte{1}, 0o644)
			require.NoError(t, err)

			var start *time.Time
			var end *time.Time

			switch ca {
			case "no filtering":

			case "filtering":
				tmp1 := time.Date(2015, 5, 19, 22, 18, 25, 427000, time.Local)
				start = &tmp1
				tmp2 := start.Add(60 * time.Minute)
				end = &tmp2

			case "start before first":
				tmp1 := time.Date(2014, 5, 19, 22, 18, 25, 427000, time.Local)
				start = &tmp1
			}

			segments, err := FindSegments(
				&conf.Path{
					Name:         "~^.*$",
					Regexp:       regexp.MustCompile("^.*$"),
					RecordPath:   filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
					RecordFormat: conf.RecordFormatFMP4,
				},
				"path1",
				start,
				end,
			)
			require.NoError(t, err)

			switch ca {
			case "no filtering", "start before first":
				require.Equal(t, []*Segment{
					{
						Fpath: filepath.Join(dir, "path1", "2015-05-19_22-15-25-000427.mp4"),
						Start: time.Date(2015, 5, 19, 22, 15, 25, 427000, time.Local),
					},
					{
						Fpath: filepath.Join(dir, "path1", "2016-05-19_22-15-25-000427.mp4"),
						Start: time.Date(2016, 5, 19, 22, 15, 25, 427000, time.Local),
					},
				}, segments)

			case "filtering":
				require.Equal(t, []*Segment{
					{
						Fpath: filepath.Join(dir, "path1", "2015-05-19_22-15-25-000427.mp4"),
						Start: time.Date(2015, 5, 19, 22, 15, 25, 427000, time.Local),
					},
				}, segments)
			}
		})
	}
}
