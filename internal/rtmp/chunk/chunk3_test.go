package chunk

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

var chunk3enc = []byte{
	0xd9, 0x1, 0x2, 0x3, 0x4,
}

var chunk3dec = Chunk3{
	ChunkStreamID: 25,
	Body:          []byte{0x01, 0x02, 0x03, 0x04},
}

func TestChunk3Read(t *testing.T) {
	var chunk3 Chunk3
	err := chunk3.Read(bytes.NewReader(chunk3enc), 4)
	require.NoError(t, err)
	require.Equal(t, chunk3dec, chunk3)
}

func TestChunk3Marshal(t *testing.T) {
	buf, err := chunk3dec.Marshal()
	require.NoError(t, err)
	require.Equal(t, chunk3enc, buf)
}
