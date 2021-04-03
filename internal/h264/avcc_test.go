package h264

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var casesAVCC = []struct {
	name string
	enc  []byte
	dec  [][]byte
}{
	{
		"single",
		[]byte{
			0x00, 0x00, 0x00, 0x03,
			0xaa, 0xbb, 0xcc,
		},
		[][]byte{
			{0xaa, 0xbb, 0xcc},
		},
	},
	{
		"multiple",
		[]byte{
			0x00, 0x00, 0x00, 0x02,
			0xaa, 0xbb,
			0x00, 0x00, 0x00, 0x02,
			0xcc, 0xdd,
			0x00, 0x00, 0x00, 0x02,
			0xee, 0xff,
		},
		[][]byte{
			{0xaa, 0xbb},
			{0xcc, 0xdd},
			{0xee, 0xff},
		},
	},
}

func TestAVCCDecode(t *testing.T) {
	for _, ca := range casesAVCC {
		t.Run(ca.name, func(t *testing.T) {
			dec, err := DecodeAVCC(ca.enc)
			require.NoError(t, err)
			require.Equal(t, ca.dec, dec)
		})
	}
}

func TestAVCCEncode(t *testing.T) {
	for _, ca := range casesAVCC {
		t.Run(ca.name, func(t *testing.T) {
			enc, err := EncodeAVCC(ca.dec)
			require.NoError(t, err)
			require.Equal(t, ca.enc, enc)
		})
	}
}

func TestAVCCDecodeError(t *testing.T) {
	for _, ca := range []struct {
		name string
		enc  []byte
	}{
		{
			"empty",
			[]byte{},
		},
		{
			"invalid length",
			[]byte{0x01},
		},
		{
			"invalid length",
			[]byte{0x00, 0x00, 0x00, 0x03},
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			_, err := DecodeAVCC(ca.enc)
			require.Error(t, err)
		})
	}
}
