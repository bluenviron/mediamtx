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
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	var timerChannel <-chan time.Time

outer:
	for {
		select {
		case event, ok := <-w.inner.Events:
			if !ok {
				break outer
			}
			currentWatchedPath, _ := filepath.EvalSymlinks(w.absolutePath)
			eventPath, _ := filepath.Abs(event.Name)
			eventPath, _ = filepath.EvalSymlinks(eventPath)

			if currentWatchedPath == "" {
				// Wait for recreation instead of reloading a missing file.
				previousWatchedPath = ""
			} else if currentWatchedPath != previousWatchedPath ||
				(eventPath == currentWatchedPath &&
					((event.Op&fsnotify.Write) == fsnotify.Write ||
						(event.Op&fsnotify.Create) == fsnotify.Create)) {
				previousWatchedPath = currentWatchedPath
				// Keep the first reload responsive, then debounce a burst at its
				// trailing edge. Never discard a newer configuration change.
				delay := additionalWait
				if time.Since(lastCalled) < minInterval {
					delay = minInterval
				}
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(delay)
				timerChannel = timer.C
			}

		case <-timerChannel:
			timerChannel = nil
			if _, err := os.Stat(w.absolutePath); err != nil {
				continue
			}
			lastCalled = time.Now()
			select {
			case w.signal <- struct{}{}:
			case <-w.terminate:
				break outer
			}

		case <-w.inner.Errors: //nolint:errcheck
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
