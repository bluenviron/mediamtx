package rtsp

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/sdp"
	"github.com/stretchr/testify/require"
)

func TestNormalizeSDPOriginNumericValues(t *testing.T) {
	t.Run("oversized", func(t *testing.T) {
		input := []byte("v=0\r\n" +
			"o=- 19599440071187478912 19599440071187478913 IN IP4 192.0.2.1\r\n" +
			"s=Stream\r\n" +
			"t=0 0\r\n")

		output, changed := normalizeSDPOriginNumericValues(input)
		require.True(t, changed)
		require.Contains(t, string(output),
			"o=- 1152695997477927296 1152695997477927297 IN IP4 192.0.2.1\r\n")

		var desc sdp.SessionDescription
		require.NoError(t, desc.Unmarshal(output))
		require.Equal(t, uint64(1152695997477927296), desc.Origin.SessionID)
		require.Equal(t, uint64(1152695997477927297), desc.Origin.SessionVersion)
	})

	t.Run("valid", func(t *testing.T) {
		input := []byte("v=0\n" +
			"o=- 123 456 IN IP4 192.0.2.1\n" +
			"s=Stream\n" +
			"t=0 0\n")

		output, changed := normalizeSDPOriginNumericValues(input)
		require.False(t, changed)
		require.Equal(t, input, output)
	})

	t.Run("invalid origin", func(t *testing.T) {
		input := []byte("o=- invalid 456 IN IP4 192.0.2.1\n")

		output, changed := normalizeSDPOriginNumericValues(input)
		require.False(t, changed)
		require.Equal(t, input, output)
	})
}
