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

	// out
	signal chan struct{}
	done   chan struct{}
}

// New allocates a ConfWatcher.
func New(confPath string) (*ConfWatcher, error) {
	if _, err := os.Stat(confPath); err != nil {
		return nil, err
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
		inner.Close()
		return nil, err
	}

	w := &ConfWatcher{
		inner:       inner,
		watchedPath: absolutePath,
		signal:      make(chan struct{}),
		done:        make(chan struct{}),
	}

	go w.run()

	return w, nil
}

// Close closes a ConfWatcher.
func (w *ConfWatcher) Close() {
	go func() {
		for range w.signal {
		}
	}()
	w.inner.Close()
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
				w.signal <- struct{}{}
			}

		case <-w.inner.Errors:
			break outer
		}
	}

	close(w.signal)
}

// Watch returns a channel that is called after the configuration file has changed.
func (w *ConfWatcher) Watch() chan struct{} {
	return w.signal
}
