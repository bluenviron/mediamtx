package httpp

import (
	"net/http"
	"sync"
)

type handlerTracker struct {
	h http.Handler

	mutex  sync.Mutex
	wg     sync.WaitGroup
	closed bool
}

func (h *handlerTracker) close() {
	h.mutex.Lock()
	h.closed = true
	h.mutex.Unlock()
	h.wg.Wait()
}

func (h *handlerTracker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mutex.Lock()
	if h.closed {
		h.mutex.Unlock()
		return
	}

	h.wg.Add(1)
	h.mutex.Unlock()

	defer h.wg.Done()

	h.h.ServeHTTP(w, r)
}
