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

var requestHeadersToRedact = map[string]struct{}{
	"Authorization":       {},
	"Cookie":              {},
	"Proxy-Authorization": {},
	"Set-Cookie":          {},
	"X-Api-Key":           {},
	"X-Auth-Token":        {},
}

func valueOrDefault(value, def string) string {
	if value != "" {
		return value
	}
	return def
}

// RequestForLog returns a string representation of a request fit for logging.
// Sensitive headers are redacted.
// Body is truncated to prevent memory exhaustion.
func RequestForLog(req *http.Request) string {
	peek, err := io.ReadAll(io.LimitReader(req.Body, maxRequestBodySizeToLog+1))
	if err != nil {
		return ""
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
				v = redactedCredential
			}
			fmt.Fprintf(&b, "%s: %s\r\n", k, v)
		}
	}

	io.WriteString(&b, "\r\n") //nolint:errcheck

	b.Write(capped)

	return b.String()
}
