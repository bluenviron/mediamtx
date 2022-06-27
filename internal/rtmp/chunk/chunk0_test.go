package chunk

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

var chunk0enc = []byte{
	0x19, 0xb1, 0xa1, 0x91, 0x0, 0x0, 0x14, 0x14,
	0x3, 0x5d, 0x17, 0x3d, 0x1, 0x2, 0x3, 0x4,
}

var chunk0dec = Chunk0{
	ChunkStreamID:   25,
	Timestamp:       11641233,
	Type:            MessageTypeCommandAMF0,
	MessageStreamID: 56432445,
	BodyLen:         20,
	Body:            []byte{0x01, 0x02, 0x03, 0x04},
}

func TestChunk0Read(t *testing.T) {
	var chunk0 Chunk0
	err := chunk0.Read(bytes.NewReader(chunk0enc), 4)
	require.NoError(t, err)
	require.Equal(t, chunk0dec, chunk0)
}

func TestChunk0Marshal(t *testing.T) {
	buf, err := chunk0dec.Marshal()
	require.NoError(t, err)
	require.Equal(t, chunk0enc, buf)
}
