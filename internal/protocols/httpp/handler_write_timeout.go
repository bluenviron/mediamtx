package httpp

import (
	"net/http"
	"time"
)

type writeTimeoutWriter struct {
	w       http.ResponseWriter
	rc      *http.ResponseController
	timeout time.Duration
}

func (w *writeTimeoutWriter) Header() http.Header {
	return w.w.Header()
}

func (w *writeTimeoutWriter) Write(p []byte) (int, error) {
	w.rc.SetWriteDeadline(time.Now().Add(w.timeout)) //nolint:errcheck
	return w.w.Write(p)
}

func (w *writeTimeoutWriter) WriteHeader(statusCode int) {
	w.rc.SetWriteDeadline(time.Now().Add(w.timeout)) //nolint:errcheck
	w.w.WriteHeader(statusCode)
}

// apply write deadline before every Write() call.
// this allows to write long responses, splitted in chunks,
// without causing timeouts.
type handlerWriteTimeout struct {
	h       http.Handler
	timeout time.Duration
}

func (h *handlerWriteTimeout) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ww := &writeTimeoutWriter{
		w:       w,
		rc:      http.NewResponseController(w),
		timeout: h.timeout,
	}

	h.h.ServeHTTP(ww, r)
}

// Flush propagates the flush to the wrapped writer. Without it, a handler that
// streams a long response (server-sent events) has its output held until it
// returns, which is exactly the case the type was written to support.
func (w *writeTimeoutWriter) Flush() {
	if f, ok := w.w.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *writeTimeoutWriter) Unwrap() http.ResponseWriter {
	return w.w
}
