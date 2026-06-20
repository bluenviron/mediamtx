package httpp

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"

	"github.com/bluenviron/mediamtx/internal/logger"
)

const (
	maxRequestBodySizeToLog = 10 * 1024
)

var requestHeadersToRedact = map[string]struct{}{
	"Authorization":       {},
	"Cookie":              {},
	"Proxy-Authorization": {},
	"Set-Cookie":          {},
	"X-Api-Key":           {},
	"X-Auth-Token":        {},
}

var requestBodyContentTypeToLog = map[string]struct{}{
	"application/sdp":                 {},
	"application/trickle-ice-sdpfrag": {},
}

func cloneRequestForLogging(r *http.Request) *http.Request {
	clone := r.Clone(r.Context())
	clone.Header = r.Header.Clone()

	for header := range requestHeadersToRedact {
		if clone.Header.Get(header) != "" {
			clone.Header.Set(header, "<redacted>")
		}
	}

	return clone
}

func dumpRequestLimited(r *http.Request) ([]byte, error) {
	peek, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodySizeToLog+1))
	if err != nil {
		return nil, err
	}

	capped := peek
	if int64(len(capped)) > maxRequestBodySizeToLog {
		capped = append([]byte(nil), capped[:maxRequestBodySizeToLog]...)
		capped = append(capped, []byte("\n\n(truncated body)\n")...)
	}

	original := r.Body
	r.Body = io.NopCloser(bytes.NewReader(capped))
	clone := cloneRequestForLogging(r)

	dump, dumpErr := httputil.DumpRequest(clone, true)

	r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(peek), original))

	return dump, dumpErr
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
	if _, ok := requestBodyContentTypeToLog[contentType]; ok {
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

// log requests and responses.
type handlerLogger struct {
	h   http.Handler
	log logger.Writer
}

func (h *handlerLogger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	byts, _ := dumpRequestLimited(r)

	h.log.Log(logger.Debug, "[conn %v] [c->s] %s", r.RemoteAddr, string(byts))

	resRecorder := &responseRecorder{w: w}

	h.h.ServeHTTP(resRecorder, r)

	h.log.Log(logger.Debug, "[conn %v] [s->c] %s", r.RemoteAddr, resRecorder.dump())
}
