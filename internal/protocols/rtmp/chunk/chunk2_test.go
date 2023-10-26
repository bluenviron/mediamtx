package chunk

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

var chunk2enc = []byte{
	0x99, 0xb1, 0xa1, 0x91, 0x1, 0x2, 0x3, 0x4,
}

var chunk2dec = Chunk2{
	ChunkStreamID:  25,
	TimestampDelta: 11641233,
	Body:           []byte{0x01, 0x02, 0x03, 0x04},
}

func TestChunk2Read(t *testing.T) {
	var chunk2 Chunk2
	err := chunk2.Read(bytes.NewReader(chunk2enc), 4)
	require.NoError(t, err)
	require.Equal(t, chunk2dec, chunk2)
}

func TestChunk2Marshal(t *testing.T) {
	buf, err := chunk2dec.Marshal()
	require.NoError(t, err)
	require.Equal(t, chunk2enc, buf)
}
