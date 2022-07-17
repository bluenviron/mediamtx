package h264conf

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var decoded = Conf{
	SPS: []byte{0x45, 0x32, 0xA3, 0x08},
	PPS: []byte{0x45, 0x34},
}

var encoded = []byte{
	0x1, 0x32, 0xa3, 0x8, 0xff, 0xe1, 0x0, 0x4, 0x45, 0x32, 0xa3, 0x8, 0x1, 0x0, 0x2, 0x45, 0x34,
}

func TestUnmarshal(t *testing.T) {
	var dec Conf
	err := dec.Unmarshal(encoded)
	require.NoError(t, err)
	require.Equal(t, decoded, dec)
}

func TestMarshal(t *testing.T) {
	enc, err := decoded.Marshal()
	require.NoError(t, err)
	require.Equal(t, encoded, enc)
}
