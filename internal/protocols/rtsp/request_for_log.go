package rtsp

import (
	"maps"

	"github.com/bluenviron/gortsplib/v5/pkg/base"
)

const (
	redactedCredential = "<redacted>"
)

func cloneHeader(header base.Header) base.Header {
	header2 := make(base.Header)
	maps.Copy(header2, header)
	return header2
}

// RequestForLog returns a string representation of a request fit for logging.
// Credentials are redacted.
func RequestForLog(req *base.Request) string {
	req2 := &base.Request{
		Method: req.Method,
		URL:    req.URL,
		Header: cloneHeader(req.Header),
		Body:   req.Body,
	}

	if _, ok := req2.Header["Authorization"]; ok {
		req2.Header["Authorization"] = base.HeaderValue{redactedCredential}
	}

	return req2.String()
}
