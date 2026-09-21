package recordcleaner

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestCleanerRunsDespiteFrequentReloads(t *testing.T) {
	timeNow = func() time.Time {
		return time.Date(2009, 5, 20, 22, 15, 25, 427000, time.Local)
	}

	dir := t.TempDir()

	err := os.Mkdir(filepath.Join(dir, "mypath"), 0o755)
	require.NoError(t, err)

	pathConfs := map[string]*conf.Path{
		"mypath": {
			Name:              "mypath",
			RecordPath:        filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
			RecordFormat:      conf.RecordFormatFMP4,
			RecordDeleteAfter: conf.Duration(2 * time.Second),
		},
	}

	c := &Cleaner{
		PathConfs: pathConfs,
		Parent:    test.NilLogger,
	}
	c.Initialize()
	defer c.Close()

	time.Sleep(200 * time.Millisecond)

	expired := filepath.Join(dir, "mypath", "2008-05-20_22-15-25-000125.mp4")
	err = os.WriteFile(expired, []byte{1}, 0o644)
	require.NoError(t, err)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c.ReloadPathConfs(pathConfs)
		time.Sleep(100 * time.Millisecond)
	}

	_, err = os.Stat(expired)
	require.Error(t, err, "expired segment must be removed despite frequent reloads")
}
