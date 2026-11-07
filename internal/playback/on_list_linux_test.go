package playback

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/recordstore"
)

// fifoSegments creates segments backed by FIFOs: opening one for reading
// blocks until a writer shows up, like opening a file on slow storage.
func fifoSegments(t *testing.T, count int) []*recordstore.Segment {
	t.Helper()
	dir := t.TempDir()
	segments := make([]*recordstore.Segment, count)

	for i := range segments {
		fpath := filepath.Join(dir, strconv.Itoa(i))
		err := syscall.Mkfifo(fpath, 0o600)
		require.NoError(t, err)
		segments[i] = &recordstore.Segment{Fpath: fpath, Start: time.Unix(int64(i), 0)}
	}

	return segments
}

// openWriters opens the write end of every FIFO that has a reader and is
// not in skip. Readers stay blocked in read() until the returned files are
// closed, then they get EOF.
func openWriters(segments []*recordstore.Segment, skip map[string]*os.File) map[string]*os.File {
	out := make(map[string]*os.File)
	for _, seg := range segments {
		if _, ok := skip[seg.Fpath]; ok {
			continue
		}

		// fails with ENXIO when the FIFO has no reader.
		f, err := os.OpenFile(seg.Fpath, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			out[seg.Fpath] = f
		}
	}
	return out
}

func closeAll(files map[string]*os.File) {
	for _, f := range files {
		f.Close()
	}
}

func TestParseSegmentsMaxParallel(t *testing.T) {
	segments := fifoSegments(t, 4*maxParallelSegmentReads)

	done := make(chan struct{})
	go func() {
		// every read fails with EOF, since nothing is written to the FIFOs.
		_, _ = parseSegments(context.Background(), segments)
		close(done)
	}()

	maxReaders := 0

	for {
		time.Sleep(50 * time.Millisecond)

		writers := openWriters(segments, nil)
		maxReaders = max(maxReaders, len(writers))
		closeAll(writers)

		select {
		case <-done:
			require.Greater(t, maxReaders, 1)
			require.LessOrEqual(t, maxReaders, maxParallelSegmentReads)
			return
		default:
		}
	}
}

func TestParseSegmentsCanceledWhileReading(t *testing.T) {
	segments := fifoSegments(t, 4*maxParallelSegmentReads)
	ctx, cancel := context.WithCancel(context.Background())

	var err error
	done := make(chan struct{})
	go func() {
		_, err = parseSegments(ctx, segments)
		close(done)
	}()

	// wait until all reads in progress are blocked.
	writers := make(map[string]*os.File)
	for len(writers) < maxParallelSegmentReads {
		time.Sleep(10 * time.Millisecond)
		maps.Copy(writers, openWriters(segments, writers))
	}

	cancel()
	closeAll(writers)

	// any read started after cancel() blocks in open() forever.
	select {
	case <-done:
		require.Error(t, err)

	case <-time.After(2 * time.Second):
		for {
			closeAll(openWriters(segments, nil))
			select {
			case <-done:
				t.Fatal("segments were read after the context was canceled")
			case <-time.After(20 * time.Millisecond):
			}
		}
	}
}
