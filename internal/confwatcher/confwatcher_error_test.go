package confwatcher

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWatcherError(t *testing.T) {
	fpath := filepath.Join(t.TempDir(), "conf.yml")
	err := os.WriteFile(fpath, []byte("{}"), 0o644)
	require.NoError(t, err)

	w := &ConfWatcher{FilePath: fpath}
	err = w.Initialize()
	require.NoError(t, err)
	defer w.Close()

	w.inner.Errors <- errors.New("queue or buffer overflow")

	// the error must not be reported as a configuration change
	select {
	case <-w.Watch():
		t.Fatal("should not happen")
	case <-time.After(500 * time.Millisecond):
	}

	// the watcher must keep working
	err = os.WriteFile(fpath, []byte("{}"), 0o644)
	require.NoError(t, err)

	select {
	case <-w.Watch():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}
