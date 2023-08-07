package httpserv

import (
	"net/http"
)

// set the Server header.
type handlerServerHeader struct {
	http.Handler
}

func (h *handlerServerHeader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Server", "mediamtx")
	h.Handler.ServeHTTP(w, r)
}
