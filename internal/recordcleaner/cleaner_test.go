package recordcleaner

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestCleaner(t *testing.T) {
	timeNow = func() time.Time {
		return time.Date(2009, 5, 20, 22, 15, 25, 427000, time.Local)
	}

	dir, err := os.MkdirTemp("", "mediamtx-cleaner")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	const specialChars = "_-+*?^$()[]{}|"

	err = os.Mkdir(filepath.Join(dir, specialChars+"_mypath"), 0o755)
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

func TestCleanerMultipleEntriesSamePath(t *testing.T) {
	timeNow = func() time.Time {
		return time.Date(2009, 5, 20, 22, 15, 25, 427000, time.Local)
	}

	dir, err := os.MkdirTemp("", "mediamtx-cleaner")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.Mkdir(filepath.Join(dir, "path1"), 0o755)
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

func TestCleanerWithSubdirectories(t *testing.T) {
	timeNow = func() time.Time {
		return time.Date(2009, 5, 20, 22, 15, 25, 427000, time.Local)
	}

	dir, err := os.MkdirTemp("", "mediamtx-cleaner")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.MkdirAll(filepath.Join(dir, "recording"), 0o755)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(dir, "recording", "compressed"), 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "recording", "2008-05-20_22-15-25-000125.mp4"), []byte{1}, 0o644)
	require.NoError(t, err)

	expiredFile := filepath.Join(dir, "recording", "compressed", "2008-05-20_22-15-25-000125_compressed.mp4")
	err = os.WriteFile(expiredFile, []byte{1}, 0o644)
	require.NoError(t, err)

	oldTime := time.Date(2008, 5, 20, 22, 15, 25, 0, time.Local)
	err = os.Chtimes(expiredFile, oldTime, oldTime)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "recording", "2009-05-20_22-15-25-000427.mp4"), []byte{1}, 0o644)
	require.NoError(t, err)

	err = os.WriteFile(
		filepath.Join(dir, "recording", "compressed", "2009-05-20_22-15-25-000427_compressed.mp4"),
		[]byte{1}, 0o644)
	require.NoError(t, err)

	c := &Cleaner{
		PathConfs: map[string]*conf.Path{
			"mypath": {
				Name:              "mypath",
				RecordPath:        filepath.Join(dir, "recording/%Y-%m-%d_%H-%M-%S-%f"),
				RecordFormat:      conf.RecordFormatFMP4,
				RecordDeleteAfter: conf.Duration(10 * time.Second),
			},
		},
		Parent: test.NilLogger,
	}
	c.Initialize()
	defer c.Close()

	time.Sleep(500 * time.Millisecond)

	_, err = os.Stat(filepath.Join(dir, "recording", "2008-05-20_22-15-25-000125.mp4"))
	require.Error(t, err, "expired file in main directory should be deleted")

	_, err = os.Stat(filepath.Join(dir, "recording", "compressed", "2008-05-20_22-15-25-000125_compressed.mp4"))
	require.Error(t, err, "expired file in subdirectory should be deleted")

	_, err = os.Stat(filepath.Join(dir, "recording", "2009-05-20_22-15-25-000427.mp4"))
	require.NoError(t, err, "current file in main directory should remain")

	_, err = os.Stat(filepath.Join(dir, "recording", "compressed", "2009-05-20_22-15-25-000427_compressed.mp4"))
	require.NoError(t, err, "current file in subdirectory should remain")

	_, err = os.Stat(filepath.Join(dir, "recording"))
	require.NoError(t, err, "main directory should still exist")

	_, err = os.Stat(filepath.Join(dir, "recording", "compressed"))
	require.NoError(t, err, "subdirectory should still exist")
}
