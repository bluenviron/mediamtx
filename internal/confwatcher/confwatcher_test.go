package confwatcher

import (
	"os"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestNoFile(t *testing.T) {
	w := &ConfWatcher{FilePath: "/nonexistent"}
	err := w.Initialize()
	require.Error(t, err)
}

func TestWrite(t *testing.T) {
	fpath, err := test.CreateTempFile([]byte("{}"))
	require.NoError(t, err)

	w := &ConfWatcher{FilePath: fpath}
	err = w.Initialize()
	require.NoError(t, err)
	defer w.Close()

	func() {
		f, err := os.Create(fpath)
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
	fpath, err := test.CreateTempFile([]byte("{}"))
	require.NoError(t, err)

	w := &ConfWatcher{FilePath: fpath}
	err = w.Initialize()
	require.NoError(t, err)
	defer w.Close()

	func() {
		f, err := os.Create(fpath)
		require.NoError(t, err)
		defer f.Close()

		_, err = f.Write([]byte("{}"))
		require.NoError(t, err)
	}()

	time.Sleep(10 * time.Millisecond)

	func() {
		f, err := os.Create(fpath)
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

	select {
	case <-time.After(500 * time.Millisecond):
	case <-w.Watch():
		t.Errorf("should not happen")
		return
	}
}

func TestDeleteCreate(t *testing.T) {
	fpath, err := test.CreateTempFile([]byte("{}"))
	require.NoError(t, err)

	w := &ConfWatcher{FilePath: fpath}
	err = w.Initialize()
	require.NoError(t, err)
	defer w.Close()

	os.Remove(fpath)
	time.Sleep(10 * time.Millisecond)

	func() {
		f, err := os.Create(fpath)
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
	fpath, err := test.CreateTempFile([]byte("{}"))
	require.NoError(t, err)

	err = os.Symlink(fpath, fpath+"-sym")
	require.NoError(t, err)

	w := &ConfWatcher{FilePath: fpath + "-sym"}
	err = w.Initialize()
	require.NoError(t, err)
	defer w.Close()

	os.Remove(fpath)

	func() {
		f, err := os.Create(fpath)
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
