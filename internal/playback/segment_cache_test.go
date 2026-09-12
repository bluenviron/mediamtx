package playback

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/recordstore"
)

func writeCacheTestFile(t *testing.T, dir string, name string, content []byte) (string, os.FileInfo) {
	fpath := filepath.Join(dir, name)

	err := os.WriteFile(fpath, content, 0o644)
	require.NoError(t, err)

	fi, err := os.Stat(fpath)
	require.NoError(t, err)

	return fpath, fi
}

func TestSegmentCache(t *testing.T) {
	t.Run("hit", func(t *testing.T) {
		fpath, fi := writeCacheTestFile(t, t.TempDir(), "seg.mp4", []byte{1, 2, 3})

		c := segmentCache{}
		c.initialize()

		init := &fmp4.Init{}
		c.set(fpath, fi, init, 50*time.Second)
		require.Equal(t, 1, c.count())

		init2, duration, ok := c.get(fpath, fi)
		require.True(t, ok)
		require.Same(t, init, init2)
		require.Equal(t, 50*time.Second, duration)
	})

	t.Run("miss", func(t *testing.T) {
		_, fi := writeCacheTestFile(t, t.TempDir(), "seg.mp4", []byte{1, 2, 3})

		c := segmentCache{}
		c.initialize()

		_, _, ok := c.get("nonexistent.mp4", fi)
		require.False(t, ok)
	})

	t.Run("size mismatch", func(t *testing.T) {
		fpath, fi := writeCacheTestFile(t, t.TempDir(), "seg.mp4", []byte{1, 2, 3})

		c := segmentCache{}
		c.initialize()
		c.set(fpath, fi, &fmp4.Init{}, 50*time.Second)

		f, err := os.OpenFile(fpath, os.O_WRONLY|os.O_APPEND, 0)
		require.NoError(t, err)
		_, err = f.Write([]byte{4})
		require.NoError(t, err)
		f.Close()

		// keep the same modification time
		err = os.Chtimes(fpath, fi.ModTime(), fi.ModTime())
		require.NoError(t, err)

		fi2, err := os.Stat(fpath)
		require.NoError(t, err)
		require.True(t, fi.ModTime().Equal(fi2.ModTime()))

		_, _, ok := c.get(fpath, fi2)
		require.False(t, ok)
		require.Equal(t, 0, c.count())
	})

	t.Run("mtime mismatch", func(t *testing.T) {
		fpath, fi := writeCacheTestFile(t, t.TempDir(), "seg.mp4", []byte{1, 2, 3})

		c := segmentCache{}
		c.initialize()
		c.set(fpath, fi, &fmp4.Init{}, 50*time.Second)

		t1 := fi.ModTime().Add(time.Hour)
		err := os.Chtimes(fpath, t1, t1)
		require.NoError(t, err)

		fi2, err := os.Stat(fpath)
		require.NoError(t, err)
		require.Equal(t, fi.Size(), fi2.Size())

		_, _, ok := c.get(fpath, fi2)
		require.False(t, ok)
		require.Equal(t, 0, c.count())
	})

	t.Run("remove", func(t *testing.T) {
		fpath, fi := writeCacheTestFile(t, t.TempDir(), "seg.mp4", []byte{1, 2, 3})

		c := segmentCache{}
		c.initialize()
		c.set(fpath, fi, &fmp4.Init{}, 50*time.Second)

		c.remove("nonexistent.mp4")
		require.Equal(t, 1, c.count())

		c.remove(fpath)
		require.Equal(t, 0, c.count())

		_, _, ok := c.get(fpath, fi)
		require.False(t, ok)
	})

	t.Run("lru eviction", func(t *testing.T) {
		dir := t.TempDir()
		pathA, fiA := writeCacheTestFile(t, dir, "a.mp4", []byte{1})
		pathB, fiB := writeCacheTestFile(t, dir, "b.mp4", []byte{2})
		pathC, fiC := writeCacheTestFile(t, dir, "c.mp4", []byte{3})
		pathD, fiD := writeCacheTestFile(t, dir, "d.mp4", []byte{4})

		c := segmentCache{maxEntries: 2}
		c.initialize()

		c.set(pathA, fiA, &fmp4.Init{}, 1*time.Second)
		c.set(pathB, fiB, &fmp4.Init{}, 2*time.Second)
		c.set(pathC, fiC, &fmp4.Init{}, 3*time.Second)
		require.Equal(t, 2, c.count())

		// the least recently used entry is evicted
		_, _, ok := c.get(pathA, fiA)
		require.False(t, ok)

		// reading an entry makes it the most recently used one
		_, _, ok = c.get(pathB, fiB)
		require.True(t, ok)

		c.set(pathD, fiD, &fmp4.Init{}, 4*time.Second)
		require.Equal(t, 2, c.count())

		_, _, ok = c.get(pathC, fiC)
		require.False(t, ok)

		_, duration, ok := c.get(pathB, fiB)
		require.True(t, ok)
		require.Equal(t, 2*time.Second, duration)

		_, duration, ok = c.get(pathD, fiD)
		require.True(t, ok)
		require.Equal(t, 4*time.Second, duration)
	})

	t.Run("update", func(t *testing.T) {
		fpath, fi := writeCacheTestFile(t, t.TempDir(), "seg.mp4", []byte{1, 2, 3})

		c := segmentCache{}
		c.initialize()

		c.set(fpath, fi, &fmp4.Init{}, 50*time.Second)

		init2 := &fmp4.Init{}
		c.set(fpath, fi, init2, 70*time.Second)
		require.Equal(t, 1, c.count())

		init3, duration, ok := c.get(fpath, fi)
		require.True(t, ok)
		require.Same(t, init2, init3)
		require.Equal(t, 70*time.Second, duration)
	})
}

func TestParseSegmentDeleted(t *testing.T) {
	fpath, fi := writeCacheTestFile(t, t.TempDir(), "seg.mp4", []byte{1, 2, 3})

	c := segmentCache{}
	c.initialize()
	c.set(fpath, fi, &fmp4.Init{}, 50*time.Second)

	err := os.Remove(fpath)
	require.NoError(t, err)

	_, err = parseSegment(&c, &recordstore.Segment{Fpath: fpath})
	require.Error(t, err)

	// the entry of a deleted segment is removed
	require.Equal(t, 0, c.count())
}
