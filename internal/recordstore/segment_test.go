package recordstore_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/recordstore"
)

func TestFindAllPathsWithSegments(t *testing.T) {
	dir := t.TempDir()

	err := os.Mkdir(filepath.Join(dir, "path1"), 0o755)
	require.NoError(t, err)

	err = os.Mkdir(filepath.Join(dir, "path2"), 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "path1", "2015-05-19_22-15-25-000427.mp4"), []byte{1}, 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "path2", "2015-07-19_22-15-25-000427.mp4"), []byte{1}, 0o644)
	require.NoError(t, err)

	paths := recordstore.FindAllPathsWithSegments(map[string]*conf.Path{
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
	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, "_2015-05-19_22-15-25-000427.mp4"), []byte{1}, 0o644)
	require.NoError(t, err)

	paths := recordstore.FindAllPathsWithSegments(map[string]*conf.Path{
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
			dir := t.TempDir()

			err := os.Mkdir(filepath.Join(dir, "path1"), 0o755)
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
				start = new(time.Date(2015, 5, 19, 22, 18, 25, 427000, time.Local))
				end = new(start.Add(60 * time.Minute))

			case "start before first":
				start = new(time.Date(2014, 5, 19, 22, 18, 25, 427000, time.Local))
			}

			segments, err := recordstore.FindSegments(
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
				require.Equal(t, []*recordstore.Segment{
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
				require.Equal(t, []*recordstore.Segment{
					{
						Fpath: filepath.Join(dir, "path1", "2015-05-19_22-15-25-000427.mp4"),
						Start: time.Date(2015, 5, 19, 22, 15, 25, 427000, time.Local),
					},
				}, segments)
			}
		})
	}
}

// BenchmarkFindSegments looks for one segment among 30 days of 5-minute
// segments, as the playback server does on every request.
func BenchmarkFindSegments(b *testing.B) {
	dir := b.TempDir()

	err := os.Mkdir(filepath.Join(dir, "mypath"), 0o755)
	require.NoError(b, err)

	first := time.Date(2015, 5, 1, 0, 0, 0, 0, time.Local)
	for i := range 30 * 24 * 12 {
		name := first.Add(time.Duration(i)*5*time.Minute).Format("2006-01-02_15-04-05-000000") + ".mp4"
		err = os.WriteFile(filepath.Join(dir, "mypath", name), nil, 0o644)
		require.NoError(b, err)
	}

	pathConf := &conf.Path{
		Name:         "mypath",
		RecordPath:   filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
		RecordFormat: conf.RecordFormatFMP4,
	}
	start := first.Add(20 * 24 * time.Hour)
	end := start.Add(30 * time.Minute)

	for b.Loop() {
		segments, err2 := recordstore.FindSegments(pathConf, "mypath", &start, &end)
		require.NoError(b, err2)
		require.NotEmpty(b, segments)
	}
}
