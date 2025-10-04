package httpp

import (
	"net/http"
)

// reject requests with empty paths.
type handlerFilterRequests struct {
	h http.Handler
}

func (h *handlerFilterRequests) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "" || r.URL.Path[0] != '/' {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	h.h.ServeHTTP(w, r)
}
