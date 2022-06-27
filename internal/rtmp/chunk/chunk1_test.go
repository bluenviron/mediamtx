package chunk

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

var chunk1enc = []byte{
	0x59, 0xb1, 0xa1, 0x91, 0x0, 0x0, 0x14, 0x14,
	0x1, 0x2, 0x3, 0x4,
}

var chunk1dec = Chunk1{
	ChunkStreamID:  25,
	TimestampDelta: 11641233,
	Type:           MessageTypeCommandAMF0,
	BodyLen:        20,
	Body:           []byte{0x01, 0x02, 0x03, 0x04},
}

func TestChunk1Read(t *testing.T) {
	var chunk1 Chunk1
	err := chunk1.Read(bytes.NewReader(chunk1enc), 4)
	require.NoError(t, err)
	require.Equal(t, chunk1dec, chunk1)
}

func TestChunk1Marshal(t *testing.T) {
	buf, err := chunk1dec.Marshal()
	require.NoError(t, err)
	require.Equal(t, chunk1enc, buf)
}
