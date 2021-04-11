package aac

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var casesADTS = []struct {
	name string
	byts []byte
	pkts []*ADTSPacket
}{
	{
		"single",
		[]byte{0xff, 0xf1, 0xc, 0x80, 0x1, 0x3c, 0x20, 0xaa, 0xbb},
		[]*ADTSPacket{
			{
				SampleRate:   48000,
				ChannelCount: 2,
				Frame:        []byte{0xaa, 0xbb},
			},
		},
	},
	{
		"multiple",
		[]byte{0xff, 0xf1, 0x10, 0x40, 0x1, 0x3c, 0x20, 0xaa, 0xbb, 0xff, 0xf1, 0xc, 0x80, 0x1, 0x3c, 0x20, 0xcc, 0xdd},
		[]*ADTSPacket{
			{
				SampleRate:   44100,
				ChannelCount: 1,
				Frame:        []byte{0xaa, 0xbb},
			},
			{
				SampleRate:   48000,
				ChannelCount: 2,
				Frame:        []byte{0xcc, 0xdd},
			},
		},
	},
}

func TestDecodeADTS(t *testing.T) {
	for _, ca := range casesADTS {
		t.Run(ca.name, func(t *testing.T) {
			pkts, err := DecodeADTS(ca.byts)
			require.NoError(t, err)
			require.Equal(t, ca.pkts, pkts)
		})
	}
}

func TestEncodeADTS(t *testing.T) {
	for _, ca := range casesADTS {
		t.Run(ca.name, func(t *testing.T) {
			byts, err := EncodeADTS(ca.pkts)
			require.NoError(t, err)
			require.Equal(t, ca.byts, byts)
		})
	}
}
