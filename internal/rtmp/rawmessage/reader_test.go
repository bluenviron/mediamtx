package rawmessage

import (
	"bufio"
	"bytes"
	"io"
	"testing"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
	"github.com/stretchr/testify/require"
)

type writableChunk interface {
	Write(w io.Writer) error
}

type sequenceEntry struct {
	chunk writableChunk
	msg   *Message
}

func TestReader(t *testing.T) {
	testSequence := func(t *testing.T, seq []sequenceEntry) {
		var buf bytes.Buffer
		r := NewReader(bufio.NewReader(&buf))

		for _, entry := range seq {
			err := entry.chunk.Write(&buf)
			require.NoError(t, err)
			msg, err := r.Read()
			require.NoError(t, err)
			require.Equal(t, entry.msg, msg)
		}
	}

	t.Run("chunk0 + chunk1", func(t *testing.T) {
		testSequence(t, []sequenceEntry{
			{
				&chunk.Chunk0{
					ChunkStreamID:   27,
					Timestamp:       18576,
					Type:            chunk.MessageTypeSetPeerBandwidth,
					MessageStreamID: 3123,
					BodyLen:         64,
					Body:            bytes.Repeat([]byte{0x02}, 64),
				},
				&Message{
					ChunkStreamID:   27,
					Timestamp:       18576,
					Type:            chunk.MessageTypeSetPeerBandwidth,
					MessageStreamID: 3123,
					Body:            bytes.Repeat([]byte{0x02}, 64),
				},
			},
			{
				&chunk.Chunk1{
					ChunkStreamID:  27,
					TimestampDelta: 15,
					Type:           chunk.MessageTypeSetPeerBandwidth,
					BodyLen:        64,
					Body:           bytes.Repeat([]byte{0x03}, 64),
				},
				&Message{
					ChunkStreamID:   27,
					Timestamp:       18576 + 15,
					Type:            chunk.MessageTypeSetPeerBandwidth,
					MessageStreamID: 3123,
					Body:            bytes.Repeat([]byte{0x03}, 64),
				},
			},
		})
	})

	t.Run("chunk0 + chunk2 + chunk3", func(t *testing.T) {
		testSequence(t, []sequenceEntry{
			{
				&chunk.Chunk0{
					ChunkStreamID:   27,
					Timestamp:       18576,
					Type:            chunk.MessageTypeSetPeerBandwidth,
					MessageStreamID: 3123,
					BodyLen:         64,
					Body:            bytes.Repeat([]byte{0x02}, 64),
				},
				&Message{
					ChunkStreamID:   27,
					Timestamp:       18576,
					Type:            chunk.MessageTypeSetPeerBandwidth,
					MessageStreamID: 3123,
					Body:            bytes.Repeat([]byte{0x02}, 64),
				},
			},
			{
				&chunk.Chunk2{
					ChunkStreamID:  27,
					TimestampDelta: 15,
					Body:           bytes.Repeat([]byte{0x03}, 64),
				},
				&Message{
					ChunkStreamID:   27,
					Timestamp:       18576 + 15,
					Type:            chunk.MessageTypeSetPeerBandwidth,
					MessageStreamID: 3123,
					Body:            bytes.Repeat([]byte{0x03}, 64),
				},
			},
			{
				&chunk.Chunk3{
					ChunkStreamID: 27,
					Body:          bytes.Repeat([]byte{0x04}, 64),
				},
				&Message{
					ChunkStreamID:   27,
					Timestamp:       18576 + 15 + 15,
					Type:            chunk.MessageTypeSetPeerBandwidth,
					MessageStreamID: 3123,
					Body:            bytes.Repeat([]byte{0x04}, 64),
				},
			},
		})
	})

	t.Run("chunk0 + chunk3", func(t *testing.T) {
		var buf bytes.Buffer
		r := NewReader(bufio.NewReader(&buf))

		err := chunk.Chunk0{
			ChunkStreamID:   27,
			Timestamp:       18576,
			Type:            chunk.MessageTypeSetPeerBandwidth,
			MessageStreamID: 3123,
			BodyLen:         192,
			Body:            bytes.Repeat([]byte{0x03}, 128),
		}.Write(&buf)
		require.NoError(t, err)

		err = chunk.Chunk3{
			ChunkStreamID: 27,
			Body:          bytes.Repeat([]byte{0x03}, 64),
		}.Write(&buf)
		require.NoError(t, err)

		msg, err := r.Read()
		require.NoError(t, err)
		require.Equal(t, &Message{
			ChunkStreamID:   27,
			Timestamp:       18576,
			Type:            chunk.MessageTypeSetPeerBandwidth,
			MessageStreamID: 3123,
			Body:            bytes.Repeat([]byte{0x03}, 192),
		}, msg)
	})
}
