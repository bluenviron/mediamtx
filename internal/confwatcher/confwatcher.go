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
	inner *fsnotify.Watcher

	// out
	signal chan struct{}
	done   chan struct{}
}

// New allocates a ConfWatcher.
func New(confPath string) (*ConfWatcher, error) {
	inner, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(confPath); err == nil {
		// use absolute path to support Darwin
		absolutePath, _ := filepath.Abs(confPath)

		err := inner.Add(absolutePath)
		if err != nil {
			inner.Close()
			return nil, err
		}
	}

	w := &ConfWatcher{
		inner:  inner,
		signal: make(chan struct{}),
		done:   make(chan struct{}),
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

outer:
	for {
		select {
		case event := <-w.inner.Events:
			if time.Since(lastCalled) < minInterval {
				continue
			}

			if (event.Op&fsnotify.Write) == fsnotify.Write ||
				(event.Op&fsnotify.Create) == fsnotify.Create {
				// wait some additional time to allow the writer to complete its job
				time.Sleep(additionalWait)

				lastCalled = time.Now()
				w.signal <- struct{}{}
			}

		case <-w.inner.Errors:
			break outer
		}
	}

	close(w.signal)
}

// Watch returns a channel that is called when the configuration file has changed.
func (w *ConfWatcher) Watch() chan struct{} {
	return w.signal
}
