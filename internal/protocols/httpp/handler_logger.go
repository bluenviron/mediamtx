package httpp

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

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

func valueOrDefault(value, def string) string {
	if value != "" {
		return value
	}
	return def
}

// this is an improvement of httputil.DumpRequest with the following changes:
// - sensitive headers are redacted
// - body is truncated to prevent memory exhaustion
func dumpRequest(req *http.Request) []byte {
	peek, err := io.ReadAll(io.LimitReader(req.Body, maxRequestBodySizeToLog+1))
	if err != nil {
		return nil
	}

	capped := peek
	if int64(len(capped)) > maxRequestBodySizeToLog {
		capped = append([]byte(nil), capped[:maxRequestBodySizeToLog]...)
		capped = append(capped, []byte("\n\n(truncated body)\n")...)
	}

	req.Body = io.NopCloser(io.MultiReader(bytes.NewReader(peek), req.Body))

	var b bytes.Buffer

	reqURI := req.RequestURI
	if reqURI == "" {
		reqURI = req.URL.RequestURI()
	}

	fmt.Fprintf(&b, "%s %s HTTP/%d.%d\r\n", valueOrDefault(req.Method, "GET"),
		reqURI, req.ProtoMajor, req.ProtoMinor)

	absRequestURI := strings.HasPrefix(req.RequestURI, "http://") || strings.HasPrefix(req.RequestURI, "https://")
	if !absRequestURI {
		host := req.Host
		if host == "" && req.URL != nil {
			host = req.URL.Host
		}
		if host != "" {
			fmt.Fprintf(&b, "Host: %s\r\n", host)
		}
	}

	keys := make([]string, 0, len(req.Header))
	for k := range req.Header {
		keys = append(keys, k)
	}

	slices.Sort(keys)

	for _, k := range keys {
		for _, v := range req.Header[k] {
			if _, ok := requestHeadersToRedact[k]; ok {
				v = "<redacted>"
			}
			fmt.Fprintf(&b, "%s: %s\r\n", k, v)
		}
	}

	io.WriteString(&b, "\r\n") //nolint:errcheck

	b.Write(capped)

	return b.Bytes()
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
	h.log.Log(logger.Debug, "[conn %v] [c->s] %s", r.RemoteAddr, dumpRequest(r))

	resRecorder := &responseRecorder{w: w}

	h.h.ServeHTTP(resRecorder, r)

	h.log.Log(logger.Debug, "[conn %v] [s->c] %s", r.RemoteAddr, resRecorder.dump())
}
