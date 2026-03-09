package push

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRTMPHostCandidates(t *testing.T) {
	for _, ca := range []struct {
		name     string
		rawURL   string
		expected []string
	}{
		{
			name:     "rtmp default port",
			rawURL:   "rtmp://example.com/app/stream",
			expected: []string{"example.com:1935"},
		},
		{
			name:     "rtmps fallback ports",
			rawURL:   "rtmps://example.com/app/stream",
			expected: []string{"example.com:443", "example.com:1936"},
		},
		{
			name:     "explicit port preserved",
			rawURL:   "rtmps://example.com:8443/app/stream",
			expected: []string{"example.com:8443"},
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			u, err := url.Parse(ca.rawURL)
			require.NoError(t, err)
			require.Equal(t, ca.expected, rtmpHostCandidates(u))
		})
	}
}
