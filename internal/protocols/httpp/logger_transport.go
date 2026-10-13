package httpp

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// LoggerTransport is a http.RoundTripper that logs outbound requests and their responses.
type LoggerTransport struct {
	Transport http.RoundTripper
	Log       logger.Writer
}

func (t *LoggerTransport) responseForLog(res *http.Response) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "HTTP/%d.%d %d %s\n", res.ProtoMajor, res.ProtoMinor, res.StatusCode, http.StatusText(res.StatusCode))
	b.WriteString(headerForLog(res.Header))
	b.WriteString("\n")

	if _, ok := responseBodyContentToLog[ParseContentType(res.Header.Get("Content-Type"))]; ok && res.Body != nil {
		// read the body, then give it back to the caller
		body, err := io.ReadAll(io.LimitReader(res.Body, maxRequestBodySizeToLog+1))
		if err == nil || len(body) > 0 {
			res.Body = &replayBody{
				Reader: io.MultiReader(bytes.NewReader(body), res.Body),
				Closer: res.Body,
			}
		}

		if len(body) > maxRequestBodySizeToLog {
			b.Write(body[:maxRequestBodySizeToLog])
			b.WriteString("\n\n(truncated body)")
		} else {
			b.Write(body)
		}
	} else if res.ContentLength > 0 {
		fmt.Fprintf(&b, "(body of %d bytes)", res.ContentLength)
	}

	return b.String()
}

// RoundTrip implements http.RoundTripper.
func (t *LoggerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// RoundTrippers must not modify the request
	req = req.Clone(req.Context())

	t.Log.Log(logger.Debug, "[c->s] %s", RequestForLog(req))

	res, err := t.Transport.RoundTrip(req)
	if err != nil {
		t.Log.Log(logger.Debug, "[s->c] request failed: %v", err)
		return nil, err
	}

	t.Log.Log(logger.Debug, "[s->c] %s", t.responseForLog(res))

	return res, nil
}
