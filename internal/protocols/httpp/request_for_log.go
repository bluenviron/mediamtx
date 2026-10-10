package httpp

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
)

const (
	maxRequestBodySizeToLog = 10 * 1024
	redactedCredential      = "<redacted>"
)

var headersToRedact = map[string]struct{}{
	"Authorization":       {},
	"Cookie":              {},
	"Proxy-Authorization": {},
	"Set-Cookie":          {},
	"X-Api-Key":           {},
	"X-Auth-Token":        {},
}

type replayBody struct {
	io.Reader
	io.Closer
}

func valueOrDefault(value, def string) string {
	if value != "" {
		return value
	}
	return def
}

func headerForLog(h http.Header) string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}

	slices.Sort(keys)

	var b bytes.Buffer

	for _, k := range keys {
		for _, v := range h[k] {
			if _, ok := headersToRedact[k]; ok {
				v = redactedCredential
			}
			fmt.Fprintf(&b, "%s: %s\r\n", k, v)
		}
	}

	return b.String()
}

// RequestForLog returns a string representation of a request fit for logging.
// Sensitive headers are redacted.
// Body is truncated to prevent memory exhaustion.
func RequestForLog(req *http.Request) string {
	var capped []byte

	if req.Body != nil && req.Body != http.NoBody {
		peek, err := io.ReadAll(io.LimitReader(req.Body, maxRequestBodySizeToLog+1))
		if err != nil {
			return ""
		}

		capped = peek
		if int64(len(capped)) > maxRequestBodySizeToLog {
			capped = append([]byte(nil), capped[:maxRequestBodySizeToLog]...)
			capped = append(capped, []byte("\n\n(truncated body)\n")...)
		}

		req.Body = &replayBody{
			Reader: io.MultiReader(bytes.NewReader(peek), req.Body),
			Closer: req.Body,
		}
	}

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

	b.WriteString(headerForLog(req.Header))

	io.WriteString(&b, "\r\n") //nolint:errcheck

	b.Write(capped)

	return b.String()
}
