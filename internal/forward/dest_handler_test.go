package forward

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveDest(t *testing.T) {
	for _, ca := range []struct {
		name     string
		dest     string
		pathName string
		matches  []string
		query    string
		expected string
	}{
		{
			name:     "path substitution",
			dest:     "rtmp://example.com/live/$MTX_PATH",
			pathName: "stream",
			expected: "rtmp://example.com/live/stream",
		},
		{
			name:     "multi digit group substitution",
			dest:     "rtmp://example.com/live/$G10",
			pathName: "stream",
			matches:  []string{"full", "g1", "g2", "g3", "g4", "g5", "g6", "g7", "g8", "g9", "g10"},
			expected: "rtmp://example.com/live/g10",
		},
		{
			name:     "query substitution",
			dest:     "rtmp://example.com/live/$MTX_PATH?$MTX_QUERY",
			pathName: "stream",
			query:    "token=abc&user=def",
			expected: "rtmp://example.com/live/stream?token=abc&user=def",
		},
		{
			name:     "combined substitutions",
			dest:     "rtmp://$G1/live/$G10?$MTX_QUERY",
			pathName: "stream",
			matches:  []string{"full", "host", "g2", "g3", "g4", "g5", "g6", "g7", "g8", "g9", "tail"},
			query:    "token=abc",
			expected: "rtmp://host/live/tail?token=abc",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			result := resolveDest(ca.dest, ca.pathName, ca.matches, ca.query)
			require.Equal(t, ca.expected, result)
		})
	}
}
