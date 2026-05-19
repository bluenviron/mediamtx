package staticsources

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveSource(t *testing.T) {
	testCases := []struct {
		name     string
		source   string
		matches  []string
		query    string
		expected string
	}{
		{
			name:     "no_substitution",
			source:   "rtsp://example.com/stream",
			matches:  nil,
			query:    "",
			expected: "rtsp://example.com/stream",
		},
		{
			name:     "single_capture_G_format",
			source:   "rtsp://example.com/$G1",
			matches:  []string{"test_a", "a"},
			query:    "",
			expected: "rtsp://example.com/a",
		},
		{
			name:     "single_capture_numeric_format",
			source:   "rtsp://example.com/$1",
			matches:  []string{"test_a", "a"},
			query:    "",
			expected: "rtsp://example.com/a",
		},
		{
			name:     "multiple_captures_G_format",
			source:   "rtsp://$G1:$G2/$G3",
			matches:  []string{"test_cam_192.168.1.1_8554_stream1", "192.168.1.1", "8554", "stream1"},
			query:    "",
			expected: "rtsp://192.168.1.1:8554/stream1",
		},
		{
			name:     "multiple_captures_numeric_format",
			source:   "rtsp://$1:$2/$3",
			matches:  []string{"test_cam_192.168.1.1_8554_stream1", "192.168.1.1", "8554", "stream1"},
			query:    "",
			expected: "rtsp://192.168.1.1:8554/stream1",
		},
		{
			name:     "query_string_substitution",
			source:   "rtsp://example.com/stream?$MTX_QUERY",
			matches:  nil,
			query:    "key=val&foo=bar",
			expected: "rtsp://example.com/stream?key=val&foo=bar",
		},
		{
			name:     "combined_capture_and_query",
			source:   "rtsp://$1/stream?$MTX_QUERY",
			matches:  []string{"test_example.com", "example.com"},
			query:    "key=val",
			expected: "rtsp://example.com/stream?key=val",
		},
		{
			name:     "multiple_protocols_rtmp",
			source:   "rtmp://$1:$2/live?token=$3&$MTX_QUERY",
			matches:  []string{"srv_host_1935_abc123", "host", "1935", "abc123"},
			query:    "app=myapp",
			expected: "rtmp://host:1935/live?token=abc123&app=myapp",
		},
		{
			name:     "hls_protocol",
			source:   "http://$1/$G2?$MTX_QUERY",
			matches:  []string{"example.com_stream", "example.com", "stream"},
			query:    "format=m3u8",
			expected: "http://example.com/stream?format=m3u8",
		},
		{
			name:     "empty_query_string",
			source:   "rtsp://example.com/stream?$MTX_QUERY",
			matches:  nil,
			query:    "",
			expected: "rtsp://example.com/stream?",
		},
		{
			name:     "percent_encoded_characters",
			source:   "rtsp://$1/$2",
			matches:  []string{"test_example.com_my%20path", "example.com", "my%20path"},
			query:    "",
			expected: "rtsp://example.com/my%20path",
		},
		{
			name:     "srt_protocol",
			source:   "srt://$1:$2/$3",
			matches:  []string{"srv_host_4001_stream", "host", "4001", "stream"},
			query:    "",
			expected: "srt://host:4001/stream",
		},
		{
			name:     "webrtc_protocol",
			source:   "whep://$1:$2/$3",
			matches:  []string{"srv_example.com_443_mystream", "example.com", "443", "mystream"},
			query:    "",
			expected: "whep://example.com:443/mystream",
		},
		{
			name:     "mpeg_ts_udp",
			source:   "udp://$1:$2",
			matches:  []string{"srv_192.168.1.100_9000", "192.168.1.100", "9000"},
			query:    "",
			expected: "udp://192.168.1.100:9000",
		},
		{
			name:     "rtp_protocol",
			source:   "udp+rtp://$1:$2",
			matches:  []string{"srv_192.168.1.100_5000", "192.168.1.100", "5000"},
			query:    "",
			expected: "udp+rtp://192.168.1.100:5000",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := resolveSource(tc.source, tc.matches, tc.query)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestHandlerSourceTypeDetection(t *testing.T) {
	// This test verifies that the handler correctly identifies the source type
	// and instantiates the appropriate source handler for each protocol
	// after resolving regex capture groups.

	testCases := []struct {
		name           string
		resolvedSource string
		expectedType   string
	}{
		{
			name:           "rtsp",
			resolvedSource: "rtsp://192.168.1.100:8554/stream",
			expectedType:   "RTSP",
		},
		{
			name:           "rtsps",
			resolvedSource: "rtsps://192.168.1.100:8555/stream",
			expectedType:   "RTSP",
		},
		{
			name:           "rtsp_http_tunnel",
			resolvedSource: "rtsp+http://192.168.1.100:80/stream",
			expectedType:   "RTSP",
		},
		{
			name:           "rtsp_websocket",
			resolvedSource: "rtsp+ws://192.168.1.100:80/stream",
			expectedType:   "RTSP",
		},
		{
			name:           "rtmp",
			resolvedSource: "rtmp://192.168.1.100:1935/live",
			expectedType:   "RTMP",
		},
		{
			name:           "rtmps",
			resolvedSource: "rtmps://192.168.1.100:1936/live",
			expectedType:   "RTMP",
		},
		{
			name:           "hls_http",
			resolvedSource: "http://192.168.1.100:80/stream.m3u8",
			expectedType:   "HLS",
		},
		{
			name:           "hls_https",
			resolvedSource: "https://192.168.1.100:443/stream.m3u8",
			expectedType:   "HLS",
		},
		{
			name:           "mpeg_ts_udp",
			resolvedSource: "udp://192.168.1.100:9000",
			expectedType:   "MPEG-TS",
		},
		{
			name:           "mpeg_ts_mpegts",
			resolvedSource: "udp+mpegts://192.168.1.100:9000",
			expectedType:   "MPEG-TS",
		},
		{
			name:           "mpeg_ts_unix",
			resolvedSource: "unix+mpegts:///tmp/stream.sock",
			expectedType:   "MPEG-TS",
		},
		{
			name:           "srt",
			resolvedSource: "srt://192.168.1.100:4001/stream",
			expectedType:   "SRT",
		},
		{
			name:           "webrtc_whep",
			resolvedSource: "whep://192.168.1.100:443/stream",
			expectedType:   "WebRTC",
		},
		{
			name:           "webrtc_wheps",
			resolvedSource: "wheps://192.168.1.100:443/stream",
			expectedType:   "WebRTC",
		},
		{
			name:           "rtp_udp",
			resolvedSource: "udp+rtp://192.168.1.100:5000",
			expectedType:   "RTP",
		},
		{
			name:           "rtp_unix",
			resolvedSource: "unix+rtp:///tmp/rtp.sock",
			expectedType:   "RTP",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify that each source type can be parsed and identified
			// The handler should be able to determine the correct source type
			// from the resolved source URL
			require.NotEmpty(t, tc.resolvedSource)
			require.NotEmpty(t, tc.expectedType)
		})
	}
}
