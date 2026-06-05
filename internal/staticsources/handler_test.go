package staticsources

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveSource(t *testing.T) {
	for _, ca := range []struct {
		name     string
		source   string
		matches  []string
		query    string
		expected string
	}{
		{
			name:     "no substitution",
			source:   "rtsp://example.com/stream",
			matches:  nil,
			query:    "",
			expected: "rtsp://example.com/stream",
		},
		{
			name:     "single capture G format",
			source:   "rtsp://example.com/$G1",
			matches:  []string{"test_a", "a"},
			query:    "",
			expected: "rtsp://example.com/a",
		},
		{
			name:     "multiple captures G format",
			source:   "rtsp://$G1:$G2/$G3",
			matches:  []string{"test_cam_192.168.1.1_8554_stream1", "192.168.1.1", "8554", "stream1"},
			query:    "",
			expected: "rtsp://192.168.1.1:8554/stream1",
		},
		{
			name:     "query string substitution",
			source:   "rtsp://example.com/stream?$MTX_QUERY",
			matches:  nil,
			query:    "key=val&foo=bar",
			expected: "rtsp://example.com/stream?key=val&foo=bar",
		},
		{
			name:     "combined capture and query",
			source:   "rtsp://$G1/stream?$MTX_QUERY",
			matches:  []string{"test_example.com", "example.com"},
			query:    "key=val",
			expected: "rtsp://example.com/stream?key=val",
		},
		{
			name:     "multiple protocols rtmp",
			source:   "rtmp://$G1:$G2/live?token=$G3&$MTX_QUERY",
			matches:  []string{"srv_host_1935_abc123", "host", "1935", "abc123"},
			query:    "app=myapp",
			expected: "rtmp://host:1935/live?token=abc123&app=myapp",
		},
		{
			name:     "hls protocol",
			source:   "http://$G1/$G2?$MTX_QUERY",
			matches:  []string{"example.com_stream", "example.com", "stream"},
			query:    "format=m3u8",
			expected: "http://example.com/stream?format=m3u8",
		},
		{
			name:     "empty query string",
			source:   "rtsp://example.com/stream?$MTX_QUERY",
			matches:  nil,
			query:    "",
			expected: "rtsp://example.com/stream?",
		},
		{
			name:     "percent encoded characters",
			source:   "rtsp://$G1/$G2",
			matches:  []string{"test_example.com_my%20path", "example.com", "my%20path"},
			query:    "",
			expected: "rtsp://example.com/my%20path",
		},
		{
			name:     "srt protocol",
			source:   "srt://$G1:$G2/$G3",
			matches:  []string{"srv_host_4001_stream", "host", "4001", "stream"},
			query:    "",
			expected: "srt://host:4001/stream",
		},
		{
			name:     "webrtc protocol",
			source:   "whep://$G1:$G2/$G3",
			matches:  []string{"srv_example.com_443_mystream", "example.com", "443", "mystream"},
			query:    "",
			expected: "whep://example.com:443/mystream",
		},
		{
			name:     "mpeg ts udp",
			source:   "udp://$G1:$G2",
			matches:  []string{"srv_192.168.1.100_9000", "192.168.1.100", "9000"},
			query:    "",
			expected: "udp://192.168.1.100:9000",
		},
		{
			name:     "rtp protocol",
			source:   "udp+rtp://$G1:$G2",
			matches:  []string{"srv_192.168.1.100_5000", "192.168.1.100", "5000"},
			query:    "",
			expected: "udp+rtp://192.168.1.100:5000",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			result := resolveSource(ca.source, ca.matches, ca.query)
			require.Equal(t, ca.expected, result)
		})
	}
}
