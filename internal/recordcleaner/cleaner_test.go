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

func TestCleanerMaxSize(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-cleaner")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.Mkdir(filepath.Join(dir, "path1"), 0o755)
	require.NoError(t, err)
	err = os.Mkdir(filepath.Join(dir, "path2"), 0o755)
	require.NoError(t, err)

	// 4 bytes each; total 16. Limit 8 keeps two newest across paths.
	err = os.WriteFile(filepath.Join(dir, "path1", "2009-05-19_22-15-25-000001.mp4"), []byte{1, 2, 3, 4}, 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "path1", "2009-05-20_22-15-25-000001.mp4"), []byte{1, 2, 3, 4}, 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "path2", "2009-05-18_22-15-25-000001.mp4"), []byte{1, 2, 3, 4}, 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "path2", "2009-05-21_22-15-25-000001.mp4"), []byte{1, 2, 3, 4}, 0o644)
	require.NoError(t, err)

	c := &Cleaner{
		PathConfs: map[string]*conf.Path{
			"path1": {
				Name:         "path1",
				RecordPath:   filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
				RecordFormat: conf.RecordFormatFMP4,
			},
			"path2": {
				Name:         "path2",
				RecordPath:   filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
				RecordFormat: conf.RecordFormatFMP4,
			},
		},
		RecordDeleteMaxSize: 8,
		Parent:              test.NilLogger,
	}
	c.Initialize()
	defer c.Close()

	time.Sleep(500 * time.Millisecond)

	_, err = os.Stat(filepath.Join(dir, "path2", "2009-05-18_22-15-25-000001.mp4"))
	require.Error(t, err)
	_, err = os.Stat(filepath.Join(dir, "path1", "2009-05-19_22-15-25-000001.mp4"))
	require.Error(t, err)
	_, err = os.Stat(filepath.Join(dir, "path1", "2009-05-20_22-15-25-000001.mp4"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "path2", "2009-05-21_22-15-25-000001.mp4"))
	require.NoError(t, err)
}

func TestCleanerMaxSizeUnderQuota(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-cleaner")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.Mkdir(filepath.Join(dir, "path1"), 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "path1", "2009-05-19_22-15-25-000001.mp4"), []byte{1, 2, 3, 4}, 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "path1", "2009-05-20_22-15-25-000001.mp4"), []byte{1, 2, 3, 4}, 0o644)
	require.NoError(t, err)

	c := &Cleaner{
		PathConfs: map[string]*conf.Path{
			"path1": {
				Name:         "path1",
				RecordPath:   filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
				RecordFormat: conf.RecordFormatFMP4,
			},
		},
		RecordDeleteMaxSize: 100,
		Parent:              test.NilLogger,
	}
	c.Initialize()
	defer c.Close()

	time.Sleep(500 * time.Millisecond)

	_, err = os.Stat(filepath.Join(dir, "path1", "2009-05-19_22-15-25-000001.mp4"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "path1", "2009-05-20_22-15-25-000001.mp4"))
	require.NoError(t, err)
}

func TestCleanerMaxSizeAndAge(t *testing.T) {
	timeNow = func() time.Time {
		return time.Date(2009, 5, 20, 22, 15, 25, 427000, time.Local)
	}
	defer func() { timeNow = time.Now }()

	dir, err := os.MkdirTemp("", "mediamtx-cleaner")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.Mkdir(filepath.Join(dir, "path1"), 0o755)
	require.NoError(t, err)

	// Age-expired (start before now-10s), then two recent ones totaling 8 bytes.
	err = os.WriteFile(filepath.Join(dir, "path1", "2008-05-20_22-15-25-000001.mp4"), []byte{1, 2, 3, 4}, 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "path1", "2009-05-20_22-15-20-000001.mp4"), []byte{1, 2, 3, 4}, 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "path1", "2009-05-20_22-15-25-000001.mp4"), []byte{1, 2, 3, 4}, 0o644)
	require.NoError(t, err)

	c := &Cleaner{
		PathConfs: map[string]*conf.Path{
			"path1": {
				Name:              "path1",
				RecordPath:        filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
				RecordFormat:      conf.RecordFormatFMP4,
				RecordDeleteAfter: conf.Duration(10 * time.Second),
			},
		},
		RecordDeleteMaxSize: 4,
		Parent:              test.NilLogger,
	}
	c.Initialize()
	defer c.Close()

	time.Sleep(500 * time.Millisecond)

	_, err = os.Stat(filepath.Join(dir, "path1", "2008-05-20_22-15-25-000001.mp4"))
	require.Error(t, err)
	_, err = os.Stat(filepath.Join(dir, "path1", "2009-05-20_22-15-20-000001.mp4"))
	require.Error(t, err)
	_, err = os.Stat(filepath.Join(dir, "path1", "2009-05-20_22-15-25-000001.mp4"))
	require.NoError(t, err)
}

func TestCleanerMaxSizeOnly(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-cleaner")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.Mkdir(filepath.Join(dir, "path1"), 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "path1", "2009-05-19_22-15-25-000001.mp4"), []byte{1, 2, 3, 4}, 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "path1", "2009-05-20_22-15-25-000001.mp4"), []byte{1, 2, 3, 4}, 0o644)
	require.NoError(t, err)

	c := &Cleaner{
		PathConfs: map[string]*conf.Path{
			"path1": {
				Name:              "path1",
				RecordPath:        filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
				RecordFormat:      conf.RecordFormatFMP4,
				RecordDeleteAfter: 0,
			},
		},
		RecordDeleteMaxSize: 4,
		Parent:              test.NilLogger,
	}
	c.Initialize()
	defer c.Close()

	time.Sleep(500 * time.Millisecond)

	_, err = os.Stat(filepath.Join(dir, "path1", "2009-05-19_22-15-25-000001.mp4"))
	require.Error(t, err)
	_, err = os.Stat(filepath.Join(dir, "path1", "2009-05-20_22-15-25-000001.mp4"))
	require.NoError(t, err)
}

func TestCleanerMaxSizeProtectsNewest(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-cleaner")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.Mkdir(filepath.Join(dir, "path1"), 0o755)
	require.NoError(t, err)

	// Single path with one large newest segment that alone exceeds the limit.
	err = os.WriteFile(filepath.Join(dir, "path1", "2009-05-19_22-15-25-000001.mp4"), []byte{1, 2}, 0o644)
	require.NoError(t, err)
	err = os.WriteFile(
		filepath.Join(dir, "path1", "2009-05-20_22-15-25-000001.mp4"),
		[]byte{1, 2, 3, 4, 5, 6, 7, 8},
		0o644,
	)
	require.NoError(t, err)

	c := &Cleaner{
		PathConfs: map[string]*conf.Path{
			"path1": {
				Name:         "path1",
				RecordPath:   filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
				RecordFormat: conf.RecordFormatFMP4,
			},
		},
		RecordDeleteMaxSize: 1,
		Parent:              test.NilLogger,
	}
	c.Initialize()
	defer c.Close()

	time.Sleep(500 * time.Millisecond)

	_, err = os.Stat(filepath.Join(dir, "path1", "2009-05-19_22-15-25-000001.mp4"))
	require.Error(t, err)
	_, err = os.Stat(filepath.Join(dir, "path1", "2009-05-20_22-15-25-000001.mp4"))
	require.NoError(t, err)
}
