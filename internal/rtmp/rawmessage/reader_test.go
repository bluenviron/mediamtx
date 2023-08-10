package rawmessage

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/rtmp/bytecounter"
	"github.com/bluenviron/mediamtx/internal/rtmp/chunk"
)

var cases = []struct {
	name       string
	messages   []*Message
	chunks     []chunk.Chunk
	chunkSizes []uint32
}{
	{
		"(chunk0) + (chunk1)",
		[]*Message{
			{
				ChunkStreamID:   27,
				Timestamp:       18576 * time.Millisecond,
				Type:            6,
				MessageStreamID: 3123,
				Body:            bytes.Repeat([]byte{0x03}, 64),
			},
			{
				ChunkStreamID:   27,
				Timestamp:       (18576 + 15) * time.Millisecond,
				Type:            5,
				MessageStreamID: 3123,
				Body:            bytes.Repeat([]byte{0x04}, 64),
			},
		},
		[]chunk.Chunk{
			&chunk.Chunk0{
				ChunkStreamID:   27,
				Timestamp:       18576,
				Type:            6,
				MessageStreamID: 3123,
				BodyLen:         64,
				Body:            bytes.Repeat([]byte{0x03}, 64),
			},
			&chunk.Chunk1{
				ChunkStreamID:  27,
				TimestampDelta: 15,
				Type:           5,
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
		"(chunk0) + (chunk2) + (chunk3)",
		[]*Message{
			{
				ChunkStreamID:   27,
				Timestamp:       18576 * time.Millisecond,
				Type:            6,
				MessageStreamID: 3123,
				Body:            bytes.Repeat([]byte{0x03}, 64),
			},
			{
				ChunkStreamID:   27,
				Timestamp:       (18576 + 15) * time.Millisecond,
				Type:            6,
				MessageStreamID: 3123,
				Body:            bytes.Repeat([]byte{0x04}, 64),
			},
			{
				ChunkStreamID:   27,
				Timestamp:       (18576 + 15 + 15) * time.Millisecond,
				Type:            6,
				MessageStreamID: 3123,
				Body:            bytes.Repeat([]byte{0x05}, 64),
			},
		},
		[]chunk.Chunk{
			&chunk.Chunk0{
				ChunkStreamID:   27,
				Timestamp:       18576,
				Type:            6,
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
		"(chunk0 + chunk3) + (chunk1 + chunk3) + (chunk2 + chunk3) + (chunk3 + chunk3)",
		[]*Message{
			{
				ChunkStreamID:   27,
				Timestamp:       18576 * time.Millisecond,
				Type:            6,
				MessageStreamID: 3123,
				Body:            bytes.Repeat([]byte{0x03}, 190),
			},
			{
				ChunkStreamID:   27,
				Timestamp:       18576 * time.Millisecond,
				Type:            6,
				MessageStreamID: 3123,
				Body:            bytes.Repeat([]byte{0x04}, 192),
			},
			{
				ChunkStreamID:   27,
				Timestamp:       (18576 + 15) * time.Millisecond,
				Type:            6,
				MessageStreamID: 3123,
				Body:            bytes.Repeat([]byte{0x05}, 192),
			},
			{
				ChunkStreamID:   27,
				Timestamp:       (18576 + 15 + 15) * time.Millisecond,
				Type:            6,
				MessageStreamID: 3123,
				Body:            bytes.Repeat([]byte{0x06}, 192),
			},
		},
		[]chunk.Chunk{
			&chunk.Chunk0{
				ChunkStreamID:   27,
				Timestamp:       18576,
				Type:            6,
				MessageStreamID: 3123,
				BodyLen:         190,
				Body:            bytes.Repeat([]byte{0x03}, 128),
			},
			&chunk.Chunk3{
				ChunkStreamID: 27,
				Body:          bytes.Repeat([]byte{0x03}, 62),
			},
			&chunk.Chunk1{
				ChunkStreamID:  27,
				TimestampDelta: 0,
				Type:           6,
				BodyLen:        192,
				Body:           bytes.Repeat([]byte{0x04}, 128),
			},
			&chunk.Chunk3{
				ChunkStreamID: 27,
				Body:          bytes.Repeat([]byte{0x04}, 64),
			},
			&chunk.Chunk2{
				ChunkStreamID:  27,
				TimestampDelta: 15,
				Body:           bytes.Repeat([]byte{0x05}, 128),
			},
			&chunk.Chunk3{
				ChunkStreamID: 27,
				Body:          bytes.Repeat([]byte{0x05}, 64),
			},
			&chunk.Chunk3{
				ChunkStreamID: 27,
				Body:          bytes.Repeat([]byte{0x06}, 128),
			},
			&chunk.Chunk3{
				ChunkStreamID: 27,
				Body:          bytes.Repeat([]byte{0x06}, 64),
			},
		},
		[]uint32{
			128,
			62,
			128,
			64,
			128,
			64,
			128,
			64,
		},
	},
}

func TestReader(t *testing.T) {
	for _, ca := range cases {
		t.Run(ca.name, func(t *testing.T) {
			var buf bytes.Buffer
			br := bytecounter.NewReader(&buf)
			r := NewReader(br, br, func(count uint32) error {
				return nil
			})

			for _, cach := range ca.chunks {
				buf2, err := cach.Marshal()
				require.NoError(t, err)
				buf.Write(buf2)
			}

			for _, camsg := range ca.messages {
				msg, err := r.Read()
				require.NoError(t, err)
				require.Equal(t, camsg, msg)
			}
		})
	}
}

func TestReaderAcknowledge(t *testing.T) {
	for _, ca := range []string{"standard", "overflow"} {
		t.Run(ca, func(t *testing.T) {
			onAckCalled := make(chan struct{})

			var buf bytes.Buffer
			bc := bytecounter.NewReader(&buf)
			r := NewReader(bc, bc, func(count uint32) error {
				close(onAckCalled)
				return nil
			})

			if ca == "overflow" {
				bc.SetCount(4294967096)
				r.lastAckCount = 4294967096
			}

			r.SetChunkSize(65536)
			r.SetWindowAckSize(100)

			buf2, err := chunk.Chunk0{
				ChunkStreamID:   27,
				Timestamp:       18576,
				Type:            6,
				MessageStreamID: 3123,
				BodyLen:         200,
				Body:            bytes.Repeat([]byte{0x03}, 200),
			}.Marshal()
			require.NoError(t, err)
			buf.Write(buf2)

			_, err = r.Read()
			require.NoError(t, err)

			<-onAckCalled
		})
	}
}
