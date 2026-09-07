package httpp

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/bluenviron/mediamtx/internal/logger"
)

var responseBodyContentToLog = map[string]struct{}{
	"application/sdp":                 {},
	"application/trickle-ice-sdpfrag": {},
}

type responseRecorder struct {
	w      http.ResponseWriter
	status int
	body   []byte
	size   int
}

func (w *responseRecorder) Header() http.Header {
	return w.w.Header()
}

func (w *responseRecorder) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}

	contentType := w.Header().Get("Content-Type")
	if _, ok := responseBodyContentToLog[contentType]; ok {
		w.body = append(w.body, b...)
	} else {
		w.size += len(b)
	}

	return w.w.Write(b)
}

func (w *responseRecorder) WriteHeader(statusCode int) {
	w.status = statusCode
	w.w.WriteHeader(statusCode)
}

func (w *responseRecorder) dump() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s %d %s\n", "HTTP/1.1", w.status, http.StatusText(w.status))
	w.w.Header().Write(&buf) //nolint:errcheck
	buf.Write([]byte("\n"))

	if w.body != nil {
		buf.Write(w.body)
	} else if w.size > 0 {
		fmt.Fprintf(&buf, "(body of %d bytes)", w.size)
	}

	return buf.String()
}

// Flush propagates the flush to the wrapped writer, so that handlers streaming
// a long response (server-sent events) are not held until they return.
func (w *responseRecorder) Flush() {
	if f, ok := w.w.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *responseRecorder) Unwrap() http.ResponseWriter {
	return w.w
}

// log requests and responses.
type handlerLogger struct {
	h   http.Handler
	log logger.Writer
}

func (h *handlerLogger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.log.Log(logger.Debug, "[conn %v] [c->s] %s", r.RemoteAddr, RequestForLog(r))

	resRecorder := &responseRecorder{w: w}

	h.h.ServeHTTP(resRecorder, r)

	h.log.Log(logger.Debug, "[conn %v] [s->c] %s", r.RemoteAddr, resRecorder.dump())
}
