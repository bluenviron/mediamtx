package httpp_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
)

var casesRequestForLog = []struct {
	name     string
	header   http.Header
	body     string
	expected string
}{
	{
		name: "small_body",
		header: http.Header{
			"Authorization":       []string{"Bearer secret-token"},
			"Cookie":              []string{"session=secret-cookie"},
			"Proxy-Authorization": []string{"Bearer proxy-secret"},
			"X-Api-Key":           []string{"secret-api-key"},
			"X-Auth-Token":        []string{"secret-auth-token"},
		},
		body: "request body",
		expected: "POST /test HTTP/1.1\r\n" +
			"Host: localhost\r\n" +
			"Authorization: <redacted>\r\n" +
			"Cookie: <redacted>\r\n" +
			"Proxy-Authorization: <redacted>\r\n" +
			"X-Api-Key: <redacted>\r\n" +
			"X-Auth-Token: <redacted>\r\n" +
			"\r\n" +
			"request body",
	},
	{
		name: "truncated_body",
		header: http.Header{
			"Authorization":       []string{"Bearer secret-token"},
			"Cookie":              []string{"session=secret-cookie"},
			"Proxy-Authorization": []string{"Bearer proxy-secret"},
			"X-Api-Key":           []string{"secret-api-key"},
			"X-Auth-Token":        []string{"secret-auth-token"},
		},
		body: strings.Repeat("a", (10 * 1024 * 2)),
		expected: "POST /test HTTP/1.1\r\n" +
			"Host: localhost\r\n" +
			"Authorization: <redacted>\r\n" +
			"Cookie: <redacted>\r\n" +
			"Proxy-Authorization: <redacted>\r\n" +
			"X-Api-Key: <redacted>\r\n" +
			"X-Auth-Token: <redacted>\r\n" +
			"\r\n" +
			strings.Repeat("a", 10*1024) +
			"\n\n(truncated body)\n",
	},
}

func TestRequestForLog(t *testing.T) {
	for _, ca := range casesRequestForLog {
		t.Run(ca.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, "http://localhost/test", strings.NewReader(ca.body))
			require.NoError(t, err)

			req.Header = ca.header

			require.Equal(t, ca.expected, httpp.RequestForLog(req))

			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Equal(t, ca.body, string(body))
		})
	}
}

func BenchmarkRequestForLog(b *testing.B) {
	for _, ca := range casesRequestForLog {
		b.Run(ca.name, func(b *testing.B) {
			req, err := http.NewRequest(http.MethodPost, "http://localhost/test", strings.NewReader(ca.body))
			if err != nil {
				b.Fatal(err)
			}

			req.Header = ca.header

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				req.Body, err = req.GetBody()
				if err != nil {
					b.Fatal(err)
				}

				_ = httpp.RequestForLog(req)
			}
		})
	}
}
