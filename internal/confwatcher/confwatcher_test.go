package confwatcher_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/confwatcher"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestNoFile(t *testing.T) {
	w := &confwatcher.ConfWatcher{FilePath: "/nonexistent"}
	err := w.Initialize()
	require.Error(t, err)
}

func TestWrite(t *testing.T) {
	fpath := test.CreateTempFile(t, []byte("{}"))

	w := &confwatcher.ConfWatcher{FilePath: fpath}
	err := w.Initialize()
	require.NoError(t, err)
	defer w.Close()

	func() {
		var f *os.File
		f, err = os.Create(fpath)
		require.NoError(t, err)
		defer f.Close()

		_, err = f.Write([]byte("{}"))
		require.NoError(t, err)
	}()

	select {
	case <-w.Watch():
	case <-time.After(500 * time.Millisecond):
		t.Errorf("timed out")
		return
	}
}

func TestWriteMultipleTimes(t *testing.T) {
	fpath := test.CreateTempFile(t, []byte("{}"))

	w := &confwatcher.ConfWatcher{FilePath: fpath}
	err := w.Initialize()
	require.NoError(t, err)
	defer w.Close()

	func() {
		f, err2 := os.Create(fpath)
		require.NoError(t, err2)
		defer f.Close()

		_, err2 = f.Write([]byte("{}"))
		require.NoError(t, err2)
	}()

	time.Sleep(10 * time.Millisecond)

	func() {
		f, err2 := os.Create(fpath)
		require.NoError(t, err2)
		defer f.Close()

		_, err2 = f.Write([]byte("{}"))
		require.NoError(t, err2)
	}()

	select {
	case <-w.Watch():
	case <-time.After(500 * time.Millisecond):
		t.Errorf("timed out")
		return
	}

	select {
	case <-time.After(500 * time.Millisecond):
	case <-w.Watch():
		t.Errorf("should not happen")
		return
	}
}

func TestDeleteCreate(t *testing.T) {
	fpath := test.CreateTempFile(t, []byte("{}"))

	w := &confwatcher.ConfWatcher{FilePath: fpath}
	err := w.Initialize()
	require.NoError(t, err)
	defer w.Close()

	os.Remove(fpath)

	time.Sleep(10 * time.Millisecond)

	func() {
		var f *os.File
		f, err = os.Create(fpath)
		require.NoError(t, err)
		defer f.Close()

		_, err = f.Write([]byte("{}"))
		require.NoError(t, err)
	}()

	select {
	case <-w.Watch():
	case <-time.After(500 * time.Millisecond):
		t.Errorf("timed out")
		return
	}
}

func TestSymlinkDeleteCreate(t *testing.T) {
	fpath := test.CreateTempFile(t, []byte("{}"))

	err := os.Symlink(fpath, fpath+"-sym")
	require.NoError(t, err)

	w := &confwatcher.ConfWatcher{FilePath: fpath + "-sym"}
	err = w.Initialize()
	require.NoError(t, err)
	defer w.Close()

	os.Remove(fpath)

	func() {
		f, err2 := os.Create(fpath)
		require.NoError(t, err2)
		defer f.Close()

		_, err2 = f.Write([]byte("{}"))
		require.NoError(t, err2)
	}()

	select {
	case <-w.Watch():
	case <-time.After(500 * time.Millisecond):
		t.Errorf("timed out")
		return
	}
}

// TestPollFallback reproduces a setup in which fsnotify never receives
// events for changes to the watched file, similarly to what happens with
// Docker bind mounts, where changes made on the host are invisible to the
// inotify watch running inside the container. fsnotify watches the parent
// directory of the file it's pointed at; here, the watched path is a
// symlink whose target lives in a different, unwatched directory, so
// writes to the target never generate an event in the watched directory.
// The change must still be detected by the periodic stat-based poll.
func TestPollFallback(t *testing.T) {
	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "conf.yml")
	err := os.WriteFile(targetPath, []byte("{}"), 0o644)
	require.NoError(t, err)

	symDir := t.TempDir()
	symPath := filepath.Join(symDir, "conf.yml")
	err = os.Symlink(targetPath, symPath)
	require.NoError(t, err)

	w := &confwatcher.ConfWatcher{FilePath: symPath}
	err = w.Initialize()
	require.NoError(t, err)
	defer w.Close()

	// give the poll loop time to record its initial file state
	time.Sleep(10 * time.Millisecond)

	err = os.WriteFile(targetPath, []byte(`{"x":1}`), 0o644)
	require.NoError(t, err)

	select {
	case <-w.Watch():
	case <-time.After(3 * time.Second):
		t.Errorf("timed out")
		return
	}
}
