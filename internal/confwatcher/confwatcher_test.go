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

// A second atomic replacement after the initial reload must still reach the
// consumer, even when it happens inside the rate limit of the first event.
func TestRapidAtomicReplacementsReloadFinalConfiguration(t *testing.T) {
	fpath := test.CreateTempFile(t, []byte("initial"))
	w := &confwatcher.ConfWatcher{FilePath: fpath}
	require.NoError(t, w.Initialize())
	defer w.Close()

	replace := func(contents string) {
		f, err := os.CreateTemp(filepath.Dir(fpath), "config-replacement-")
		require.NoError(t, err)
		_, err = f.WriteString(contents)
		require.NoError(t, err)
		require.NoError(t, f.Close())
		require.NoError(t, os.Rename(f.Name(), fpath))
	}
	await := func(wanted string) {
		select {
		case <-w.Watch():
			contents, err := os.ReadFile(fpath)
			require.NoError(t, err)
			require.Equal(t, wanted, string(contents))
		case <-time.After(3 * time.Second):
			t.Fatalf("configuration %q was never reloaded", wanted)
		}
	}

	replace("camera-added")
	await("camera-added")
	replace("camera-updated")
	replace("camera-removed")
	await("camera-removed")
}

func TestPendingReloadDeleteCreate(t *testing.T) {
	fpath := test.CreateTempFile(t, []byte("initial"))
	w := &confwatcher.ConfWatcher{FilePath: fpath}
	require.NoError(t, w.Initialize())
	defer w.Close()

	write := func(contents string) {
		t.Helper()
		require.NoError(t, os.WriteFile(fpath, []byte(contents), 0o600))
	}
	await := func() {
		t.Helper()
		select {
		case <-w.Watch():
		case <-time.After(3 * time.Second):
			t.Fatal("configuration was never reloaded")
		}
	}

	write("first")
	await()
	write("second")
	// Allow the write event to arm the trailing timer, then delete the file.
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, os.Remove(fpath))
	select {
	case <-w.Watch():
		t.Fatal("missing configuration triggered a reload")
	case <-time.After(1200 * time.Millisecond):
	}

	write("recreated")
	await()
}

func TestCloseWithPendingReload(t *testing.T) {
	fpath := test.CreateTempFile(t, []byte("initial"))
	w := &confwatcher.ConfWatcher{FilePath: fpath}
	require.NoError(t, w.Initialize())
	require.NoError(t, os.WriteFile(fpath, []byte("updated"), 0o600))
	// No receiver: Close must also interrupt delivery of a pending notification.
	time.Sleep(100 * time.Millisecond)
	done := make(chan struct{})
	go func() {
		w.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("close blocked on a pending reload")
	}
}
