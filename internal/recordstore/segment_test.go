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

func TestFindSegments(t *testing.T) {
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

	segments, err := FindSegments(
		&conf.Path{
			Name:         "~^.*$",
			Regexp:       regexp.MustCompile("^.*$"),
			RecordPath:   filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
			RecordFormat: conf.RecordFormatFMP4,
		},
		"path1",
	)
	require.NoError(t, err)

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
}

func TestFindSegmentsInTimespan(t *testing.T) {
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

	segments, err := FindSegmentsInTimespan(
		&conf.Path{
			Name:         "~^.*$",
			Regexp:       regexp.MustCompile("^.*$"),
			RecordPath:   filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
			RecordFormat: conf.RecordFormatFMP4,
		},
		"path1",
		time.Date(2015, 5, 19, 22, 18, 25, 427000, time.Local),
		60*time.Minute,
	)
	require.NoError(t, err)

	require.Equal(t, []*Segment{
		{
			Fpath: filepath.Join(dir, "path1", "2015-05-19_22-15-25-000427.mp4"),
			Start: time.Date(2015, 5, 19, 22, 15, 25, 427000, time.Local),
		},
	}, segments)
}
