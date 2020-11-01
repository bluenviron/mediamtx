package confwatcher

import (
	"os"
	"time"

	"github.com/fsnotify/fsnotify"
)

type ConfWatcher struct {
	inner *fsnotify.Watcher

	// out
	signal chan struct{}
	done   chan struct{}
}

func New(confPath string) (*ConfWatcher, error) {
	inner, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(confPath); err == nil {
		err := inner.Add(confPath)
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

outer:
	for {
		select {
		case event := <-w.inner.Events:
			if (event.Op & fsnotify.Write) == fsnotify.Write {
				// wait some additional time to avoid EOF
				time.Sleep(10 * time.Millisecond)
				w.signal <- struct{}{}
			}

		case <-w.inner.Errors:
			break outer
		}
	}

	close(w.signal)
}

func (w *ConfWatcher) Watch() chan struct{} {
	return w.signal
}
