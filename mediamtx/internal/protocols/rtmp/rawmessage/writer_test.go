package rawmessage

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/bytecounter"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/chunk"
	"github.com/stretchr/testify/require"
)

func chunkBodySize(ch chunk.Chunk) uint32 {
	switch ch := ch.(type) {
	case *chunk.Chunk0:
		return uint32(len(ch.Body))
	case *chunk.Chunk1:
		return uint32(len(ch.Body))
	case *chunk.Chunk2:
		return uint32(len(ch.Body))
	case *chunk.Chunk3:
		return uint32(len(ch.Body))
	}
	return 0
}

func chunkHasExtendedTimestamp(ch chunk.Chunk) bool {
	switch ch := ch.(type) {
	case *chunk.Chunk0:
		return ch.Timestamp >= 0xFFFFFF
	case *chunk.Chunk1:
		return ch.TimestampDelta >= 0xFFFFFF
	case *chunk.Chunk2:
		return ch.TimestampDelta >= 0xFFFFFF
	case *chunk.Chunk3:
		return false
	}
	return false
}

func TestWriter(t *testing.T) {
	for _, ca := range cases {
		t.Run(ca.name, func(t *testing.T) {
			var buf bytes.Buffer
			bc := bytecounter.NewWriter(&buf)
			w := NewWriter(bc, bc, true)

			for _, msg := range ca.messages {
				err := w.Write(msg)
				require.NoError(t, err)
			}

			hasExtendedTimestamp := false

			for _, cach := range ca.chunks {
				ch := reflect.New(reflect.TypeOf(cach).Elem()).Interface().(chunk.Chunk)
				err := ch.Read(&buf, chunkBodySize(cach), hasExtendedTimestamp)
				require.NoError(t, err)
				require.Equal(t, cach, ch)
				hasExtendedTimestamp = chunkHasExtendedTimestamp(cach)
			}
		})
	}
}

func TestWriterAcknowledge(t *testing.T) {
	for _, ca := range []string{"standard", "overflow"} {
		t.Run(ca, func(t *testing.T) {
			var buf bytes.Buffer
			bc := bytecounter.NewWriter(&buf)
			w := NewWriter(bc, bc, true)

			if ca == "overflow" {
				bc.SetCount(4294967096)
				w.ackValue = 4294967096
			}

			w.SetChunkSize(65536)
			w.SetWindowAckSize(100)

			err := w.Write(&Message{
				ChunkStreamID:   27,
				Timestamp:       18576 * time.Millisecond,
				Type:            6,
				MessageStreamID: 3123,
				Body:            bytes.Repeat([]byte{0x03}, 200),
			})
			require.NoError(t, err)

			err = w.Write(&Message{
				ChunkStreamID:   27,
				Timestamp:       18576 * time.Millisecond,
				Type:            6,
				MessageStreamID: 3123,
				Body:            bytes.Repeat([]byte{0x03}, 200),
			})
			require.EqualError(t, err, "no acknowledge received within window")
		})
	}
}
