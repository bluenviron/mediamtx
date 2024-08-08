package chunk

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

var cases = []struct {
	name                 string
	enc                  []byte
	bodyLen              uint32
	hasExtendedTimestamp bool
	dec                  Chunk
}{
	{
		"chunk0 standard",
		[]byte{
			0x19, 0xb1, 0xa1, 0x91, 0x0, 0x0, 0x14, 0x14,
			0x3, 0x5d, 0x17, 0x3d, 0x1, 0x2, 0x3, 0x4,
		},
		4,
		false,
		&Chunk0{
			ChunkStreamID:   25,
			Timestamp:       11641233,
			Type:            20,
			MessageStreamID: 56432445,
			BodyLen:         20,
			Body:            []byte{1, 2, 3, 4},
		},
	},
	{
		"chunk0 extended timestamp",
		[]byte{
			0x19, 0xff, 0xff, 0xff, 0x00, 0x00, 0x14, 0x0f,
			0x00, 0x31, 0x84, 0xb2, 0xff, 0x34, 0x86, 0xa2,
			0x05, 0x06, 0x07, 0x08,
		},
		4,
		false,
		&Chunk0{
			ChunkStreamID:   25,
			Timestamp:       0xFF3486a2,
			Type:            15,
			MessageStreamID: 3245234,
			BodyLen:         20,
			Body:            []byte{5, 6, 7, 8},
		},
	},
	{
		"chunk1 standard",
		[]byte{
			0x59, 0xb1, 0xa1, 0x91, 0x0, 0x0, 0x14, 0x14,
			0x1, 0x2, 0x3, 0x4,
		},
		4,
		false,
		&Chunk1{
			ChunkStreamID:  25,
			TimestampDelta: 11641233,
			Type:           20,
			BodyLen:        20,
			Body:           []byte{1, 2, 3, 4},
		},
	},
	{
		"chunk1 extended timestamp",
		[]byte{
			0x59, 0xff, 0xff, 0xff, 0x00, 0x00, 0x14, 0x14,
			0xff, 0x88, 0x4b, 0x6c, 0x05, 0x06, 0x07, 0x08,
		},
		4,
		false,
		&Chunk1{
			ChunkStreamID:  25,
			TimestampDelta: 0xFF884B6C,
			Type:           20,
			BodyLen:        20,
			Body:           []byte{5, 6, 7, 8},
		},
	},
	{
		"chunk2 standard",
		[]byte{
			0x99, 0xb1, 0xa1, 0x91, 0x1, 0x2, 0x3, 0x4,
		},
		4,
		false,
		&Chunk2{
			ChunkStreamID:  25,
			TimestampDelta: 11641233,
			Body:           []byte{1, 2, 3, 4},
		},
	},
	{
		"chunk2 extended timestamp",
		[]byte{
			0x99, 0xff, 0xff, 0xff, 0xff, 0xaa, 0xbb, 0xcc,
			0x05, 0x06, 0x07, 0x08,
		},
		4,
		false,
		&Chunk2{
			ChunkStreamID:  25,
			TimestampDelta: 0xFFAABBCC,
			Body:           []byte{5, 6, 7, 8},
		},
	},
	{
		"chunk3 standard",
		[]byte{
			0xd9, 0x1, 0x2, 0x3, 0x4,
		},
		4,
		false,
		&Chunk3{
			ChunkStreamID: 25,
			Body:          []byte{1, 2, 3, 4},
		},
	},
	{
		"chunk3 extended timestamp",
		[]byte{
			0xd9, 0x00, 0x00, 0x00, 0x00, 0x05, 0x06, 0x07,
			0x08,
		},
		4,
		true,
		&Chunk3{
			ChunkStreamID: 25,
			Body:          []byte{5, 6, 7, 8},
		},
	},
}

func TestChunkRead(t *testing.T) {
	for _, ca := range cases {
		t.Run(ca.name, func(t *testing.T) {
			chunk := reflect.New(reflect.TypeOf(ca.dec).Elem()).Interface().(Chunk)
			err := chunk.Read(bytes.NewReader(ca.enc), ca.bodyLen, ca.hasExtendedTimestamp)
			require.NoError(t, err)
			require.Equal(t, ca.dec, chunk)
		})
	}
}

func TestChunkMarshal(t *testing.T) {
	for _, ca := range cases {
		t.Run(ca.name, func(t *testing.T) {
			buf, err := ca.dec.Marshal(ca.hasExtendedTimestamp)
			require.NoError(t, err)
			require.Equal(t, ca.enc, buf)
		})
	}
}

func FuzzChunk0Read(f *testing.F) {
	f.Fuzz(func(_ *testing.T, b []byte) {
		var chunk Chunk0
		err := chunk.Read(bytes.NewReader(b), 65536, false)
		if err == nil {
			chunk.Marshal(false) //nolint:errcheck
		}
	})
}

func FuzzChunk1Read(f *testing.F) {
	f.Fuzz(func(_ *testing.T, b []byte) {
		var chunk Chunk1
		err := chunk.Read(bytes.NewReader(b), 65536, false)
		if err == nil {
			chunk.Marshal(false) //nolint:errcheck
		}
	})
}

func FuzzChunk2Read(f *testing.F) {
	f.Fuzz(func(_ *testing.T, b []byte) {
		var chunk Chunk2
		err := chunk.Read(bytes.NewReader(b), 65536, false)
		if err == nil {
			chunk.Marshal(false) //nolint:errcheck
		}
	})
}

func FuzzChunk3Read(f *testing.F) {
	f.Fuzz(func(_ *testing.T, b []byte) {
		var chunk Chunk3
		err := chunk.Read(bytes.NewReader(b), 65536, true)
		if err == nil {
			chunk.Marshal(false) //nolint:errcheck
		}
	})
}
