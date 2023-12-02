package record

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
)

func TestCleaner(t *testing.T) {
	timeNow = func() time.Time {
		return time.Date(2009, 0o5, 20, 22, 15, 25, 427000, time.UTC)
	}

	dir, err := os.MkdirTemp("", "mediamtx-cleaner")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	recordPath := filepath.Join(dir, "_-+*?^$()[]{}|_%path/%Y-%m-%d_%H-%M-%S-%f")

	err = os.Mkdir(filepath.Join(dir, "_-+*?^$()[]{}|_mypath"), 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "_-+*?^$()[]{}|_mypath", "2008-05-20_22-15-25-000125.mp4"), []byte{1}, 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "_-+*?^$()[]{}|_mypath", "2009-05-20_22-15-25-000427.mp4"), []byte{1}, 0o644)
	require.NoError(t, err)

	c := &Cleaner{
		Entries: []CleanerEntry{{
			SegmentPathFormat: recordPath,
			Format:            conf.RecordFormatFMP4,
			DeleteAfter:       10 * time.Second,
		}},
		Parent: nilLogger{},
	}
	c.Initialize()
	defer c.Close()

	time.Sleep(500 * time.Millisecond)

	_, err = os.Stat(filepath.Join(dir, "_-+*?^$()[]{}|_mypath", "2008-05-20_22-15-25-000125.mp4"))
	require.Error(t, err)

	_, err = os.Stat(filepath.Join(dir, "_-+*?^$()[]{}|_mypath", "2009-05-20_22-15-25-000427.mp4"))
	require.NoError(t, err)
}
