package recordcleaner

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestCleaner(t *testing.T) {
	timeNow = func() time.Time {
		return time.Date(2009, 5, 20, 22, 15, 25, 427000, time.Local)
	}

	dir := t.TempDir()

	const specialChars = "_-+*?^$()[]{}|"

	err := os.Mkdir(filepath.Join(dir, specialChars+"_mypath"), 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, specialChars+"_mypath", "2008-05-20_22-15-25-000125.mp4"), []byte{1}, 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, specialChars+"_mypath", "2009-05-20_22-15-25-000427.mp4"), []byte{1}, 0o644)
	require.NoError(t, err)

	c := &Cleaner{
		PathConfs: map[string]*conf.Path{
			"~^.*$": {
				Name:              "~^.*$",
				Regexp:            regexp.MustCompile("^.*$"),
				RecordPath:        filepath.Join(dir, specialChars+"_%path/%Y-%m-%d_%H-%M-%S-%f"),
				RecordFormat:      conf.RecordFormatFMP4,
				RecordDeleteAfter: conf.Duration(10 * time.Second),
			},
		},
		Parent: test.NilLogger,
	}
	c.Initialize()
	defer c.Close()

	time.Sleep(500 * time.Millisecond)

	_, err = os.Stat(filepath.Join(dir, specialChars+"_mypath", "2008-05-20_22-15-25-000125.mp4"))
	require.Error(t, err)

	_, err = os.Stat(filepath.Join(dir, specialChars+"_mypath", "2009-05-20_22-15-25-000427.mp4"))
	require.NoError(t, err)
}

// Segments of several paths in a single directory, next to unrelated files and directories.
func TestCleanerSharedDirectory(t *testing.T) {
	timeNow = func() time.Time {
		return time.Date(2009, 5, 20, 22, 15, 25, 427000, time.Local)
	}

	dir := t.TempDir()

	recordPath := filepath.Join(dir, "%path_%Y%m%d_%H%M%S_%f")

	pathConfs := map[string]*conf.Path{
		"cam1": {
			Name:              "cam1",
			RecordPath:        recordPath,
			RecordFormat:      conf.RecordFormatFMP4,
			RecordDeleteAfter: conf.Duration(10 * time.Second),
		},
		"cam_1": {
			Name:              "cam_1",
			RecordPath:        recordPath,
			RecordFormat:      conf.RecordFormatFMP4,
			RecordDeleteAfter: conf.Duration(10 * 24 * time.Hour),
		},
		"cam2": {
			Name:         "cam2",
			RecordPath:   recordPath,
			RecordFormat: conf.RecordFormatFMP4,
		},
		"~^other.*$": {
			Name:              "~^other.*$",
			Regexp:            regexp.MustCompile("^other.*$"),
			RecordPath:        recordPath,
			RecordFormat:      conf.RecordFormatFMP4,
			RecordDeleteAfter: conf.Duration(10 * time.Second),
		},
	}

	files := map[string]bool{ // file -> must survive
		"cam1_20090519_221525_000427.mp4":    false,
		"cam1_20090520_221525_000427.mp4":    true,
		"cam_1_20090519_221525_000427.mp4":   true,
		"cam2_20090519_221525_000427.mp4":    true,
		"other1_20090519_221525_000427.mp4":  false,
		"unknown_20090519_221525_000427.mp4": true,
		"cam1/20090519_22/snapshot.jpg":      true,
		"cam1/20090519_22/thumbs/thumb.jpg":  true,
		"notes.txt":                          true,
	}

	for name := range files {
		fpath := filepath.Join(dir, name)
		err := os.MkdirAll(filepath.Dir(fpath), 0o755)
		require.NoError(t, err)
		err = os.WriteFile(fpath, []byte{1}, 0o644)
		require.NoError(t, err)
	}

	err := os.MkdirAll(filepath.Join(dir, "cam1", "20090518_22", "thumbs"), 0o755)
	require.NoError(t, err)

	c := &Cleaner{
		PathConfs: pathConfs,
		Parent:    test.NilLogger,
	}
	c.Initialize()
	defer c.Close()

	time.Sleep(500 * time.Millisecond)

	for name, survives := range files {
		_, err = os.Stat(filepath.Join(dir, name))
		if survives {
			require.NoError(t, err, name)
		} else {
			require.Error(t, err, name)
		}
	}

	_, err = os.Stat(filepath.Join(dir, "cam1", "20090518_22"))
	require.Error(t, err, "empty directories are removed, children before parents")
}

func TestCleanerMultipleEntriesSamePath(t *testing.T) {
	timeNow = func() time.Time {
		return time.Date(2009, 5, 20, 22, 15, 25, 427000, time.Local)
	}

	dir := t.TempDir()

	err := os.Mkdir(filepath.Join(dir, "path1"), 0o755)
	require.NoError(t, err)

	err = os.Mkdir(filepath.Join(dir, "path2"), 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "path1", "2009-05-19_22-15-25-000427.mp4"), []byte{1}, 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "path2", "2009-05-19_22-15-25-000427.mp4"), []byte{1}, 0o644)
	require.NoError(t, err)

	c := &Cleaner{
		PathConfs: map[string]*conf.Path{
			"path1": {
				Name:              "path1",
				RecordPath:        filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
				RecordFormat:      conf.RecordFormatFMP4,
				RecordDeleteAfter: conf.Duration(10 * time.Second),
			},
			"path2": {
				Name:              "path2",
				RecordPath:        filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
				RecordFormat:      conf.RecordFormatFMP4,
				RecordDeleteAfter: conf.Duration(10 * 24 * time.Hour),
			},
		},
		Parent: test.NilLogger,
	}
	c.Initialize()
	defer c.Close()

	time.Sleep(500 * time.Millisecond)

	_, err = os.Stat(filepath.Join(dir, "path1", "2009-05-19_22-15-25-000427.mp4"))
	require.Error(t, err)

	_, err = os.Stat(filepath.Join(dir, "path1"))
	require.Error(t, err, "testing")

	_, err = os.Stat(filepath.Join(dir, "path2", "2009-05-19_22-15-25-000427.mp4"))
	require.NoError(t, err)
}
