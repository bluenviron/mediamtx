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

// A cleaning pass over a large recording directory can take hours.
// Reloading the configuration must not wait for it, since it is called by the core loop.
func TestCleanerReloadDoesNotWaitForPass(t *testing.T) {
	passStarted := make(chan struct{})
	releasePass := make(chan struct{})
	first := true

	timeNow = func() time.Time {
		// timeNow is called at the start of every pass; block the first one
		// to simulate a pass that walks a large directory.
		if first {
			first = false
			close(passStarted)
			<-releasePass
		}
		return time.Date(2009, 5, 20, 22, 15, 25, 427000, time.Local)
	}
	defer func() { timeNow = time.Now }()

	dir := t.TempDir()

	newPathConfs := func(deleteAfter time.Duration) map[string]*conf.Path {
		return map[string]*conf.Path{
			"mypath": {
				Name:              "mypath",
				RecordPath:        filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
				RecordFormat:      conf.RecordFormatFMP4,
				RecordDeleteAfter: conf.Duration(deleteAfter),
			},
		}
	}

	c := &Cleaner{
		PathConfs: newPathConfs(10 * time.Second),
		Parent:    test.NilLogger,
	}
	c.Initialize()
	defer c.Close()
	defer close(releasePass)

	<-passStarted

	reloaded := make(chan struct{})
	go func() {
		c.ReloadPathConfs(newPathConfs(20 * time.Second))
		c.ReloadPathConfs(newPathConfs(30 * time.Second))
		close(reloaded)
	}()

	select {
	case <-reloaded:
	case <-time.After(1 * time.Second):
		t.Fatal("ReloadPathConfs blocked until the cleaning pass ended")
	}
}

// Closing the cleaner, which happens when recordDeleteAfter is disabled on all paths,
// must stop a pass in progress instead of waiting for it.
func TestCleanerCloseStopsPass(t *testing.T) {
	passStarted := make(chan struct{})
	releasePass := make(chan struct{})
	first := true

	timeNow = func() time.Time {
		if first {
			first = false
			close(passStarted)
			<-releasePass
		}
		return time.Date(2009, 5, 20, 22, 15, 25, 427000, time.Local)
	}
	defer func() { timeNow = time.Now }()

	dir := t.TempDir()

	err := os.Mkdir(filepath.Join(dir, "mypath"), 0o755)
	require.NoError(t, err)

	expired := filepath.Join(dir, "mypath", "2008-05-20_22-15-25-000125.mp4")
	err = os.WriteFile(expired, []byte{1}, 0o644)
	require.NoError(t, err)

	c := &Cleaner{
		PathConfs: map[string]*conf.Path{
			"mypath": {
				Name:              "mypath",
				RecordPath:        filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
				RecordFormat:      conf.RecordFormatFMP4,
				RecordDeleteAfter: conf.Duration(10 * time.Second),
			},
		},
		Parent: test.NilLogger,
	}
	c.Initialize()

	<-passStarted

	closed := make(chan struct{})
	go func() {
		c.Close()
		close(closed)
	}()

	// let Close() cancel the context before the pass walks the directory.
	time.Sleep(100 * time.Millisecond)
	close(releasePass)

	select {
	case <-closed:
	case <-time.After(1 * time.Second):
		t.Fatal("Close blocked")
	}

	_, err = os.Stat(expired)
	require.NoError(t, err, "a closed cleaner must not keep removing files")
}
