package handshake

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

var c1s1enc = bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 1536/4)

var c1s1dec = C1S1{
	Time:    16909060,
	Version: 16909060,
	Data:    bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 1536/4-2),
}

func TestC1S1Read(t *testing.T) {
	var c1s1 C1S1
	err := c1s1.Read((bytes.NewReader(c1s1enc)))
	require.NoError(t, err)
	require.Equal(t, c1s1dec, c1s1)
}

func TestC1S1Write(t *testing.T) {
	var buf bytes.Buffer
	err := c1s1dec.Write(&buf)
	require.NoError(t, err)
	require.Equal(t, c1s1enc, buf.Bytes())
}
