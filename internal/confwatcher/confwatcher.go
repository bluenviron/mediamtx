// Package confwatcher contains a configuration watcher.
package confwatcher

import (
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	minInterval    = 1 * time.Second
	additionalWait = 10 * time.Millisecond

	// pollInterval is the interval at which the watched file is periodically
	// stat()ed, as a backstop for setups (for example Docker bind mounts)
	// where fsnotify never receives events for changes made on the host.
	pollInterval = 1 * time.Second
)

// fileState holds the file attributes used to detect changes through polling.
type fileState struct {
	modTime time.Time
	size    int64
}

func statFileState(path string) (fileState, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return fileState{}, false
	}
	return fileState{modTime: info.ModTime(), size: info.Size()}, true
}

func (s fileState) equal(other fileState) bool {
	return s.modTime.Equal(other.modTime) && s.size == other.size
}

// ConfWatcher is a configuration file watcher.
type ConfWatcher struct {
	FilePath string

	inner        *fsnotify.Watcher
	absolutePath string

	// in
	terminate chan struct{}

	// out
	signal chan struct{}
	done   chan struct{}
}

// Initialize initializes a ConfWatcher.
func (w *ConfWatcher) Initialize() error {
	if _, err := os.Stat(w.FilePath); err != nil {
		return err
	}

	var err error
	w.inner, err = fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	// use absolute paths to support Darwin
	w.absolutePath, _ = filepath.Abs(w.FilePath)
	parentPath := filepath.Dir(w.absolutePath)

	err = w.inner.Add(parentPath)
	if err != nil {
		w.inner.Close() //nolint:errcheck
		return err
	}

	w.terminate = make(chan struct{})
	w.signal = make(chan struct{})
	w.done = make(chan struct{})

	go w.run()

	return nil
}

// Close closes a ConfWatcher.
func (w *ConfWatcher) Close() {
	close(w.terminate)
	<-w.done
}

func (w *ConfWatcher) run() {
	defer close(w.done)

	var lastCalled time.Time
	previousWatchedPath, _ := filepath.EvalSymlinks(w.absolutePath)
	lastState, _ := statFileState(w.absolutePath)

	pollTicker := time.NewTicker(pollInterval)
	defer pollTicker.Stop()

outer:
	for {
		select {
		case event := <-w.inner.Events:
			currentWatchedPath, _ := filepath.EvalSymlinks(w.absolutePath)
			eventPath, _ := filepath.Abs(event.Name)
			eventPath, _ = filepath.EvalSymlinks(eventPath)

			if currentWatchedPath == "" {
				// watched file was removed; wait for write event to trigger reload
				previousWatchedPath = ""
			} else if currentWatchedPath != previousWatchedPath ||
				(eventPath == currentWatchedPath &&
					((event.Op&fsnotify.Write) == fsnotify.Write ||
						(event.Op&fsnotify.Create) == fsnotify.Create)) {
				// wait some additional time to allow the writer to complete its job
				time.Sleep(additionalWait)
				previousWatchedPath = currentWatchedPath
				lastState, _ = statFileState(w.absolutePath)

				// keep lastState in sync even when the signal itself is throttled,
				// otherwise the poll ticker compares against a stale lastState and
				// fires a spurious extra reload once minInterval elapses
				if time.Since(lastCalled) < minInterval {
					continue
				}

				lastCalled = time.Now()

				select {
				case w.signal <- struct{}{}:
				case <-w.terminate:
					break outer
				}
			}

		case <-pollTicker.C:
			if time.Since(lastCalled) < minInterval {
				continue
			}

			currentState, exists := statFileState(w.absolutePath)
			if !exists {
				// watched file was removed; wait for it to reappear before reloading
				lastState = fileState{}
				continue
			}

			if !currentState.equal(lastState) {
				// wait some additional time to allow the writer to complete its job
				time.Sleep(additionalWait)
				lastState, _ = statFileState(w.absolutePath)
				lastCalled = time.Now()

				select {
				case w.signal <- struct{}{}:
				case <-w.terminate:
					break outer
				}
			}

		case <-w.inner.Errors:
			break outer

		case <-w.terminate:
			break outer
		}
	}

	close(w.signal)
	w.inner.Close() //nolint:errcheck
}

// Watch returns a channel that is called after the configuration file has changed.
func (w *ConfWatcher) Watch() chan struct{} {
	return w.signal
}
