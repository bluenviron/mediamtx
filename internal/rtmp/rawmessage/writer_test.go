package rawmessage

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/bytecounter"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
	"github.com/stretchr/testify/require"
)

func TestWriter(t *testing.T) {
	for _, ca := range []struct {
		name       string
		messages   []*Message
		chunks     []chunk.Chunk
		chunkSizes []uint32
	}{
		{
			"chunk0 + chunk1",
			[]*Message{
				{
					ChunkStreamID:   27,
					Timestamp:       18576 * time.Millisecond,
					Type:            chunk.MessageTypeSetPeerBandwidth,
					MessageStreamID: 3123,
					Body:            bytes.Repeat([]byte{0x03}, 64),
				},
				{
					ChunkStreamID:   27,
					Timestamp:       (18576 + 15) * time.Millisecond,
					Type:            chunk.MessageTypeSetWindowAckSize,
					MessageStreamID: 3123,
					Body:            bytes.Repeat([]byte{0x04}, 64),
				},
			},
			[]chunk.Chunk{
				&chunk.Chunk0{
					ChunkStreamID:   27,
					Timestamp:       18576,
					Type:            chunk.MessageTypeSetPeerBandwidth,
					MessageStreamID: 3123,
					BodyLen:         64,
					Body:            bytes.Repeat([]byte{0x03}, 64),
				},
				&chunk.Chunk1{
					ChunkStreamID:  27,
					TimestampDelta: 15,
					Type:           chunk.MessageTypeSetWindowAckSize,
					BodyLen:        64,
					Body:           bytes.Repeat([]byte{0x04}, 64),
				},
			},
			[]uint32{
				128,
				128,
			},
		},
		{
			"chunk0 + chunk2 + chunk3",
			[]*Message{
				{
					ChunkStreamID:   27,
					Timestamp:       18576 * time.Millisecond,
					Type:            chunk.MessageTypeSetPeerBandwidth,
					MessageStreamID: 3123,
					Body:            bytes.Repeat([]byte{0x03}, 64),
				},
				{
					ChunkStreamID:   27,
					Timestamp:       (18576 + 15) * time.Millisecond,
					Type:            chunk.MessageTypeSetPeerBandwidth,
					MessageStreamID: 3123,
					Body:            bytes.Repeat([]byte{0x04}, 64),
				},
				{
					ChunkStreamID:   27,
					Timestamp:       (18576 + 15 + 15) * time.Millisecond,
					Type:            chunk.MessageTypeSetPeerBandwidth,
					MessageStreamID: 3123,
					Body:            bytes.Repeat([]byte{0x05}, 64),
				},
			},
			[]chunk.Chunk{
				&chunk.Chunk0{
					ChunkStreamID:   27,
					Timestamp:       18576,
					Type:            chunk.MessageTypeSetPeerBandwidth,
					MessageStreamID: 3123,
					BodyLen:         64,
					Body:            bytes.Repeat([]byte{0x03}, 64),
				},
				&chunk.Chunk2{
					ChunkStreamID:  27,
					TimestampDelta: 15,
					Body:           bytes.Repeat([]byte{0x04}, 64),
				},
				&chunk.Chunk3{
					ChunkStreamID: 27,
					Body:          bytes.Repeat([]byte{0x05}, 64),
				},
			},
			[]uint32{
				128,
				64,
				64,
			},
		},
		{
			"chunk0 + chunk3 + chunk2 + chunk3",
			[]*Message{
				{
					ChunkStreamID:   27,
					Timestamp:       18576 * time.Millisecond,
					Type:            chunk.MessageTypeSetPeerBandwidth,
					MessageStreamID: 3123,
					Body:            bytes.Repeat([]byte{0x03}, 192),
				},
				{
					ChunkStreamID:   27,
					Timestamp:       18591 * time.Millisecond,
					Type:            chunk.MessageTypeSetPeerBandwidth,
					MessageStreamID: 3123,
					Body:            bytes.Repeat([]byte{0x04}, 192),
				},
			},
			[]chunk.Chunk{
				&chunk.Chunk0{
					ChunkStreamID:   27,
					Timestamp:       18576,
					Type:            chunk.MessageTypeSetPeerBandwidth,
					MessageStreamID: 3123,
					BodyLen:         192,
					Body:            bytes.Repeat([]byte{0x03}, 128),
				},
				&chunk.Chunk3{
					ChunkStreamID: 27,
					Body:          bytes.Repeat([]byte{0x03}, 64),
				},
				&chunk.Chunk2{
					ChunkStreamID:  27,
					TimestampDelta: 15,
					Body:           bytes.Repeat([]byte{0x04}, 128),
				},
				&chunk.Chunk3{
					ChunkStreamID: 27,
					Body:          bytes.Repeat([]byte{0x04}, 64),
				},
			},
			[]uint32{
				128,
				64,
				128,
				64,
			},
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := NewWriter(bytecounter.NewWriter(&buf), true)

			for _, msg := range ca.messages {
				err := w.Write(msg)
				require.NoError(t, err)
			}

			for i, cach := range ca.chunks {
				ch := reflect.New(reflect.TypeOf(cach).Elem()).Interface().(chunk.Chunk)
				err := ch.Read(&buf, ca.chunkSizes[i])
				require.NoError(t, err)
				require.Equal(t, cach, ch)
			}
		})
	}
}

func TestWriterAcknowledge(t *testing.T) {
	for _, ca := range []string{"standard", "overflow"} {
		t.Run(ca, func(t *testing.T) {
			var buf bytes.Buffer
			bcw := bytecounter.NewWriter(&buf)
			w := NewWriter(bcw, true)

			if ca == "overflow" {
				bcw.SetCount(4294967096)
				w.ackValue = 4294967096
			}

			w.SetChunkSize(65536)
			w.SetWindowAckSize(100)

			err := w.Write(&Message{
				ChunkStreamID:   27,
				Timestamp:       18576 * time.Millisecond,
				Type:            chunk.MessageTypeSetPeerBandwidth,
				MessageStreamID: 3123,
				Body:            bytes.Repeat([]byte{0x03}, 200),
			})
			require.NoError(t, err)

			err = w.Write(&Message{
				ChunkStreamID:   27,
				Timestamp:       18576 * time.Millisecond,
				Type:            chunk.MessageTypeSetPeerBandwidth,
				MessageStreamID: 3123,
				Body:            bytes.Repeat([]byte{0x03}, 200),
			})
			require.EqualError(t, err, "no acknowledge received within window")
		})
	}
}
