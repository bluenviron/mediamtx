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
	name      string
	header    http.Header
	body      string
	chunked   bool
	expected  string
	readsBody bool
}{
	{
		name: "small_body",
		header: http.Header{
			"Authorization":       []string{"Bearer secret-token"},
			"Content-Type":        []string{"application/sdp"},
			"Cookie":              []string{"session=secret-cookie"},
			"Proxy-Authorization": []string{"Bearer proxy-secret"},
			"X-Api-Key":           []string{"secret-api-key"},
			"X-Auth-Token":        []string{"secret-auth-token"},
		},
		body: "request body",
		expected: "POST /test HTTP/1.1\r\n" +
			"Host: localhost\r\n" +
			"Authorization: <redacted>\r\n" +
			"Content-Type: application/sdp\r\n" +
			"Cookie: <redacted>\r\n" +
			"Proxy-Authorization: <redacted>\r\n" +
			"X-Api-Key: <redacted>\r\n" +
			"X-Auth-Token: <redacted>\r\n" +
			"\r\n" +
			"request body",
		readsBody: true,
	},
	{
		name: "truncated_body",
		header: http.Header{
			"Authorization":       []string{"Bearer secret-token"},
			"Content-Type":        []string{"application/sdp"},
			"Cookie":              []string{"session=secret-cookie"},
			"Proxy-Authorization": []string{"Bearer proxy-secret"},
			"X-Api-Key":           []string{"secret-api-key"},
			"X-Auth-Token":        []string{"secret-auth-token"},
		},
		body: strings.Repeat("a", (10 * 1024 * 2)),
		expected: "POST /test HTTP/1.1\r\n" +
			"Host: localhost\r\n" +
			"Authorization: <redacted>\r\n" +
			"Content-Type: application/sdp\r\n" +
			"Cookie: <redacted>\r\n" +
			"Proxy-Authorization: <redacted>\r\n" +
			"X-Api-Key: <redacted>\r\n" +
			"X-Auth-Token: <redacted>\r\n" +
			"\r\n" +
			strings.Repeat("a", 10*1024) +
			"\n\n(truncated body)\n",
		readsBody: true,
	},
	{
		name: "sdp_with_params",
		header: http.Header{
			"Content-Type": []string{"application/sdp; x=y"},
		},
		body: "v",
		expected: "POST /test HTTP/1.1\r\n" +
			"Host: localhost\r\n" +
			"Content-Type: application/sdp; x=y\r\n" +
			"\r\n" +
			"v",
		readsBody: true,
	},
	{
		name: "json_body",
		header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		body: `{"pass":"DUMMYSECRET"}`,
		expected: "POST /test HTTP/1.1\r\n" +
			"Host: localhost\r\n" +
			"Content-Type: application/json\r\n" +
			"\r\n" +
			"(body of 22 bytes)",
	},
	{
		name: "json_with_params",
		header: http.Header{
			"Content-Type": []string{"application/json; charset=utf-8"},
		},
		body: `{"pass":"DUMMYSECRET"}`,
		expected: "POST /test HTTP/1.1\r\n" +
			"Host: localhost\r\n" +
			"Content-Type: application/json; charset=utf-8\r\n" +
			"\r\n" +
			"(body of 22 bytes)",
	},
	{
		name: "form_urlencoded",
		header: http.Header{
			"Content-Type": []string{"application/x-www-form-urlencoded"},
		},
		body: `{"pass":"DUMMYSECRET"}`,
		expected: "POST /test HTTP/1.1\r\n" +
			"Host: localhost\r\n" +
			"Content-Type: application/x-www-form-urlencoded\r\n" +
			"\r\n" +
			"(body of 22 bytes)",
	},
	{
		name:   "no_content_type",
		header: http.Header{},
		body:   "x",
		expected: "POST /test HTTP/1.1\r\n" +
			"Host: localhost\r\n" +
			"\r\n" +
			"(body of 1 bytes)",
	},
	{
		name:   "empty_body",
		header: http.Header{},
		expected: "POST /test HTTP/1.1\r\n" +
			"Host: localhost\r\n" +
			"\r\n",
	},
	{
		name: "chunked_json",
		header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		body:    `{"pass":"DUMMYSECRET"}`,
		chunked: true,
		expected: "POST /test HTTP/1.1\r\n" +
			"Host: localhost\r\n" +
			"Content-Type: application/json\r\n" +
			"\r\n",
	},
}

type countingReader struct {
	r     io.Reader
	reads int
}

func (r *countingReader) Read(p []byte) (int, error) {
	r.reads++
	return r.r.Read(p)
}

func TestRequestForLog(t *testing.T) {
	for _, ca := range casesRequestForLog {
		t.Run(ca.name, func(t *testing.T) {
			cr := &countingReader{r: strings.NewReader(ca.body)}
			req, err := http.NewRequest(http.MethodPost, "http://localhost/test", cr)
			require.NoError(t, err)

			req.Header = ca.header
			req.ContentLength = int64(len(ca.body))
			if ca.chunked {
				req.ContentLength = -1
			}

			out := httpp.RequestForLog(req)
			require.Equal(t, ca.expected, out)
			require.NotContains(t, out, "DUMMYSECRET")
			require.Equal(t, ca.readsBody, cr.reads != 0)

			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Equal(t, ca.body, string(body))
		})
	}
}

func TestRequestForLogNoBody(t *testing.T) {
	for _, ca := range []struct {
		name string
		body io.ReadCloser
	}{
		{"nil", nil},
		{"no_body", http.NoBody},
	} {
		t.Run(ca.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, "http://localhost/test", nil)
			require.NoError(t, err)

			req.Header.Set("Content-Type", "application/sdp")
			req.Body = ca.body

			require.Equal(t, "POST /test HTTP/1.1\r\n"+
				"Host: localhost\r\n"+
				"Content-Type: application/sdp\r\n"+
				"\r\n", httpp.RequestForLog(req))
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
			if ca.chunked {
				req.ContentLength = -1
			}

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
