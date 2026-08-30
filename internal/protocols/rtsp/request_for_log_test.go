package rtsp

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/stretchr/testify/require"
)

var casesRequestForLog = []struct {
	name     string
	req      *base.Request
	expected string
}{
	{
		"main",
		&base.Request{
			Method: base.Describe,
			URL:    &base.URL{Scheme: "rtsp", Host: "localhost", Path: "/test"},
			Header: base.Header{
				"OtherHeader":   []string{"value"},
				"Authorization": []string{"Basic secret"},
			},
			Body: []byte("request body"),
		},
		"DESCRIBE rtsp://localhost/test RTSP/1.0\r\n" +
			"Authorization: <redacted>\r\n" +
			"Content-Length: 12\r\n" +
			"OtherHeader: value\r\n" +
			"\r\nrequest body",
	},
}

func TestRequestForLog(t *testing.T) {
	for _, ca := range casesRequestForLog {
		t.Run(ca.name, func(t *testing.T) {
			s := RequestForLog(ca.req)
			require.Equal(t, ca.expected, s)
		})
	}
}
