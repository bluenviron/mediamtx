package rawmessage

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/bytecounter"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
)

type sequenceEntry struct {
	chunk chunk.Chunk
	msg   *Message
}

func TestReader(t *testing.T) {
	testSequence := func(t *testing.T, seq []sequenceEntry) {
		var buf bytes.Buffer
		bcr := bytecounter.NewReader(&buf)
		r := NewReader(bcr, func(count uint32) error {
			return nil
		})

		for _, entry := range seq {
			buf2, err := entry.chunk.Marshal()
			require.NoError(t, err)
			buf.Write(buf2)
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
		bcr := bytecounter.NewReader(&buf)
		r := NewReader(bcr, func(count uint32) error {
			return nil
		})

		buf2, err := chunk.Chunk0{
			ChunkStreamID:   27,
			Timestamp:       18576,
			Type:            chunk.MessageTypeSetPeerBandwidth,
			MessageStreamID: 3123,
			BodyLen:         192,
			Body:            bytes.Repeat([]byte{0x03}, 128),
		}.Marshal()
		require.NoError(t, err)
		buf.Write(buf2)

		buf2, err = chunk.Chunk3{
			ChunkStreamID: 27,
			Body:          bytes.Repeat([]byte{0x03}, 64),
		}.Marshal()
		require.NoError(t, err)
		buf.Write(buf2)

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

func TestReaderAcknowledge(t *testing.T) {
	onAckCalled := make(chan struct{})

	var buf bytes.Buffer
	bcr := bytecounter.NewReader(&buf)
	r := NewReader(bcr, func(count uint32) error {
		close(onAckCalled)
		return nil
	})

	r.SetWindowAckSize(100)

	for i := 0; i < 2; i++ {
		buf2, err := chunk.Chunk0{
			ChunkStreamID:   27,
			Timestamp:       18576,
			Type:            chunk.MessageTypeSetPeerBandwidth,
			MessageStreamID: 3123,
			BodyLen:         64,
			Body:            bytes.Repeat([]byte{0x03}, 64),
		}.Marshal()
		require.NoError(t, err)
		buf.Write(buf2)
	}

	for i := 0; i < 2; i++ {
		_, err := r.Read()
		require.NoError(t, err)
	}

	<-onAckCalled
}
