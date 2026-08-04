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
			name:     "combined substitutions",
			dest:     "rtmp://$G1/live/$G10",
			pathName: "stream",
			matches:  []string{"full", "host", "g2", "g3", "g4", "g5", "g6", "g7", "g8", "g9", "tail"},
			expected: "rtmp://host/live/tail",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			result := resolveDest(ca.dest, ca.pathName, ca.matches)
			require.Equal(t, ca.expected, result)
		})
	}
}
