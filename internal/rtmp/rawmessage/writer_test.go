package rawmessage

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/rtmp/bytecounter"
	"github.com/bluenviron/mediamtx/internal/rtmp/chunk"
	"github.com/stretchr/testify/require"
)

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
