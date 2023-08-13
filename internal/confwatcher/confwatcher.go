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
	inner       *fsnotify.Watcher
	watchedPath string

	// in
	terminate chan struct{}

	// out
	signal chan struct{}
	done   chan struct{}
}

// New allocates a ConfWatcher.
func New(confPath string) (*ConfWatcher, error) {
	if _, err := os.Stat(confPath); err != nil {
		if confPath == "mediamtx.yml" {
			confPath = "rtsp-simple-server.yml"
			if _, err := os.Stat(confPath); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	inner, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	// use absolute paths to support Darwin
	absolutePath, _ := filepath.Abs(confPath)
	parentPath := filepath.Dir(absolutePath)

	err = inner.Add(parentPath)
	if err != nil {
		inner.Close() //nolint:errcheck
		return nil, err
	}

	w := &ConfWatcher{
		inner:       inner,
		watchedPath: absolutePath,
		terminate:   make(chan struct{}),
		signal:      make(chan struct{}),
		done:        make(chan struct{}),
	}

	go w.run()

	return w, nil
}

// Close closes a ConfWatcher.
func (w *ConfWatcher) Close() {
	close(w.terminate)
	<-w.done
}

func (w *ConfWatcher) run() {
	defer close(w.done)

	var lastCalled time.Time
	previousWatchedPath, _ := filepath.EvalSymlinks(w.watchedPath)

outer:
	for {
		select {
		case event := <-w.inner.Events:
			if time.Since(lastCalled) < minInterval {
				continue
			}

			currentWatchedPath, _ := filepath.EvalSymlinks(w.watchedPath)
			eventPath, _ := filepath.Abs(event.Name)

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
