package confwatcher

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func writeTempFile(byts []byte) (string, error) {
	tmpf, err := ioutil.TempFile(os.TempDir(), "confwatcher-")
	if err != nil {
		return "", err
	}
	defer tmpf.Close()

	_, err = tmpf.Write(byts)
	if err != nil {
		return "", err
	}

	return tmpf.Name(), nil
}

func TestNoFile(t *testing.T) {
	_, err := New("/nonexistent")
	require.Error(t, err)
}

func TestWrite(t *testing.T) {
	fpath, err := writeTempFile([]byte("{}"))
	require.NoError(t, err)

	w, err := New(fpath)
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
	fpath, err := writeTempFile([]byte("{}"))
	require.NoError(t, err)

	w, err := New(fpath)
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
	fpath, err := writeTempFile([]byte("{}"))
	require.NoError(t, err)

	w, err := New(fpath)
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
	fpath, err := writeTempFile([]byte("{}"))
	require.NoError(t, err)

	err = os.Symlink(fpath, fpath+"-sym")
	require.NoError(t, err)

	w, err := New(fpath + "-sym")
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
