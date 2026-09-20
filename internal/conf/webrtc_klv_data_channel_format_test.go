package conf

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWebRTCKLVDataChannelFormatUnmarshalJSON(t *testing.T) {
	for _, ca := range []struct {
		input    string
		expected WebRTCKLVDataChannelFormat
		ok       bool
	}{
		{`"raw"`, WebRTCKLVDataChannelFormatRaw, true},
		{`"timed"`, WebRTCKLVDataChannelFormatTimed, true},
		{`"invalid"`, "", false},
	} {
		var actual WebRTCKLVDataChannelFormat
		err := actual.UnmarshalJSON([]byte(ca.input))
		if ca.ok {
			require.NoError(t, err)
			require.Equal(t, ca.expected, actual)
		} else {
			require.Error(t, err)
		}
	}
}
