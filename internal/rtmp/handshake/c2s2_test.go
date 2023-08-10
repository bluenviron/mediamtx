package handshake

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

var c2s2enc = bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 1536/4)

var c2s2dec = C2S2{
	Time:  16909060,
	Time2: 16909060,
	Data:  bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 1536/4-2),
}

func TestC2S2Read(t *testing.T) {
	var c2s2 C2S2
	err := c2s2.Read((bytes.NewReader(c2s2enc)))
	require.NoError(t, err)
	require.Equal(t, c2s2dec, c2s2)
}

func TestC2S2Write(t *testing.T) {
	var buf bytes.Buffer
	err := c2s2dec.Write(&buf)
	require.NoError(t, err)
	require.Equal(t, c2s2enc, buf.Bytes())
}
