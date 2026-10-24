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
