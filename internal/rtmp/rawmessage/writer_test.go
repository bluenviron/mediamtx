package rawmessage

import (
	"bytes"
	"testing"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/bytecounter"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
	"github.com/stretchr/testify/require"
)

func TestWriter(t *testing.T) {
	t.Run("chunk0 + chunk1", func(t *testing.T) {
		var buf bytes.Buffer
		w := NewWriter(bytecounter.NewWriter(&buf))

		err := w.Write(&Message{
			ChunkStreamID:   27,
			Timestamp:       18576,
			Type:            chunk.MessageTypeSetPeerBandwidth,
			MessageStreamID: 3123,
			Body:            bytes.Repeat([]byte{0x03}, 64),
		})
		require.NoError(t, err)

		var c0 chunk.Chunk0
		err = c0.Read(&buf, 128)
		require.NoError(t, err)
		require.Equal(t, chunk.Chunk0{
			ChunkStreamID:   27,
			Timestamp:       18576,
			Type:            chunk.MessageTypeSetPeerBandwidth,
			MessageStreamID: 3123,
			BodyLen:         64,
			Body:            bytes.Repeat([]byte{0x03}, 64),
		}, c0)

		err = w.Write(&Message{
			ChunkStreamID:   27,
			Timestamp:       18576 + 15,
			Type:            chunk.MessageTypeSetWindowAckSize,
			MessageStreamID: 3123,
			Body:            bytes.Repeat([]byte{0x04}, 64),
		})
		require.NoError(t, err)

		var c1 chunk.Chunk1
		err = c1.Read(&buf, 128)
		require.NoError(t, err)
		require.Equal(t, chunk.Chunk1{
			ChunkStreamID:  27,
			TimestampDelta: 15,
			Type:           chunk.MessageTypeSetWindowAckSize,
			BodyLen:        64,
			Body:           bytes.Repeat([]byte{0x04}, 64),
		}, c1)
	})

	t.Run("chunk0 + chunk2 + chunk3", func(t *testing.T) {
		var buf bytes.Buffer
		w := NewWriter(bytecounter.NewWriter(&buf))

		err := w.Write(&Message{
			ChunkStreamID:   27,
			Timestamp:       18576,
			Type:            chunk.MessageTypeSetPeerBandwidth,
			MessageStreamID: 3123,
			Body:            bytes.Repeat([]byte{0x03}, 64),
		})
		require.NoError(t, err)

		var c0 chunk.Chunk0
		err = c0.Read(&buf, 128)
		require.NoError(t, err)
		require.Equal(t, chunk.Chunk0{
			ChunkStreamID:   27,
			Timestamp:       18576,
			Type:            chunk.MessageTypeSetPeerBandwidth,
			MessageStreamID: 3123,
			BodyLen:         64,
			Body:            bytes.Repeat([]byte{0x03}, 64),
		}, c0)

		err = w.Write(&Message{
			ChunkStreamID:   27,
			Timestamp:       18576 + 15,
			Type:            chunk.MessageTypeSetPeerBandwidth,
			MessageStreamID: 3123,
			Body:            bytes.Repeat([]byte{0x04}, 64),
		})
		require.NoError(t, err)

		var c2 chunk.Chunk2
		err = c2.Read(&buf, 64)
		require.NoError(t, err)
		require.Equal(t, chunk.Chunk2{
			ChunkStreamID:  27,
			TimestampDelta: 15,
			Body:           bytes.Repeat([]byte{0x04}, 64),
		}, c2)

		err = w.Write(&Message{
			ChunkStreamID:   27,
			Timestamp:       18576 + 15 + 15,
			Type:            chunk.MessageTypeSetPeerBandwidth,
			MessageStreamID: 3123,
			Body:            bytes.Repeat([]byte{0x05}, 64),
		})
		require.NoError(t, err)

		var c3 chunk.Chunk3
		err = c3.Read(&buf, 64)
		require.NoError(t, err)
		require.Equal(t, chunk.Chunk3{
			ChunkStreamID: 27,
			Body:          bytes.Repeat([]byte{0x05}, 64),
		}, c3)
	})

	t.Run("chunk0 + chunk3", func(t *testing.T) {
		var buf bytes.Buffer
		w := NewWriter(bytecounter.NewWriter(&buf))

		err := w.Write(&Message{
			ChunkStreamID:   27,
			Timestamp:       18576,
			Type:            chunk.MessageTypeSetPeerBandwidth,
			MessageStreamID: 3123,
			Body:            bytes.Repeat([]byte{0x03}, 192),
		})
		require.NoError(t, err)

		var c0 chunk.Chunk0
		err = c0.Read(&buf, 128)
		require.NoError(t, err)
		require.Equal(t, chunk.Chunk0{
			ChunkStreamID:   27,
			Timestamp:       18576,
			Type:            chunk.MessageTypeSetPeerBandwidth,
			MessageStreamID: 3123,
			BodyLen:         192,
			Body:            bytes.Repeat([]byte{0x03}, 128),
		}, c0)

		var c3 chunk.Chunk3
		err = c3.Read(&buf, 64)
		require.NoError(t, err)
		require.Equal(t, chunk.Chunk3{
			ChunkStreamID: 27,
			Body:          bytes.Repeat([]byte{0x03}, 64),
		}, c3)
	})
}

func TestWriterAcknowledge(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(bytecounter.NewWriter(&buf))

	w.SetWindowAckSize(100)

	for i := 0; i < 2; i++ {
		err := w.Write(&Message{
			ChunkStreamID:   27,
			Timestamp:       18576,
			Type:            chunk.MessageTypeSetPeerBandwidth,
			MessageStreamID: 3123,
			Body:            bytes.Repeat([]byte{0x03}, 64),
		})
		require.NoError(t, err)
	}

	err := w.Write(&Message{
		ChunkStreamID:   27,
		Timestamp:       18576,
		Type:            chunk.MessageTypeSetPeerBandwidth,
		MessageStreamID: 3123,
		Body:            bytes.Repeat([]byte{0x03}, 64),
	})
	require.EqualError(t, err, "no acknowledge received within window")
}
