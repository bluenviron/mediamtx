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
)

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

outer:
	for {
		select {
		case event := <-w.inner.Events:
			if time.Since(lastCalled) < minInterval {
				continue
			}

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
