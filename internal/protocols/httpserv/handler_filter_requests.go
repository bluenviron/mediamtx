package httpserv

import (
	"net/http"
)

// reject requests with empty paths.
type handlerFilterRequests struct {
	http.Handler
}

func (h *handlerFilterRequests) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "" || r.URL.Path[0] != '/' {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	h.Handler.ServeHTTP(w, r)
}
