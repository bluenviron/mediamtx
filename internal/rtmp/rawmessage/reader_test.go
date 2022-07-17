package rawmessage

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/bytecounter"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
)

func TestReader(t *testing.T) {
	type sequenceEntry struct {
		chunk chunk.Chunk
		msg   *Message
	}

	for _, ca := range []struct {
		name     string
		sequence []sequenceEntry
	}{
		{
			"chunk0 + chunk1",
			[]sequenceEntry{
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
						Timestamp:       18576 * time.Millisecond,
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
						Timestamp:       (18576 + 15) * time.Millisecond,
						Type:            chunk.MessageTypeSetPeerBandwidth,
						MessageStreamID: 3123,
						Body:            bytes.Repeat([]byte{0x03}, 64),
					},
				},
			},
		},
		{
			"chunk0 + chunk2 + chunk3",
			[]sequenceEntry{
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
						Timestamp:       18576 * time.Millisecond,
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
						Timestamp:       (18576 + 15) * time.Millisecond,
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
						Timestamp:       (18576 + 15 + 15) * time.Millisecond,
						Type:            chunk.MessageTypeSetPeerBandwidth,
						MessageStreamID: 3123,
						Body:            bytes.Repeat([]byte{0x04}, 64),
					},
				},
			},
		},
		{
			"chunk0 + chunk3 + chunk2 + chunk3",
			[]sequenceEntry{
				{
					&chunk.Chunk0{
						ChunkStreamID:   27,
						Timestamp:       18576,
						Type:            chunk.MessageTypeSetPeerBandwidth,
						MessageStreamID: 3123,
						BodyLen:         192,
						Body:            bytes.Repeat([]byte{0x03}, 128),
					},
					nil,
				},
				{
					&chunk.Chunk3{
						ChunkStreamID: 27,
						Body:          bytes.Repeat([]byte{0x03}, 64),
					},
					&Message{
						ChunkStreamID:   27,
						Timestamp:       18576 * time.Millisecond,
						Type:            chunk.MessageTypeSetPeerBandwidth,
						MessageStreamID: 3123,
						Body:            bytes.Repeat([]byte{0x03}, 192),
					},
				},
				{
					&chunk.Chunk2{
						ChunkStreamID:  27,
						TimestampDelta: 15,
						Body:           bytes.Repeat([]byte{0x04}, 128),
					},
					nil,
				},
				{
					&chunk.Chunk3{
						ChunkStreamID: 27,
						Body:          bytes.Repeat([]byte{0x04}, 64),
					},
					&Message{
						ChunkStreamID:   27,
						Timestamp:       18591 * time.Millisecond,
						Type:            chunk.MessageTypeSetPeerBandwidth,
						MessageStreamID: 3123,
						Body:            bytes.Repeat([]byte{0x04}, 192),
					},
				},
			},
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			var buf bytes.Buffer
			bcr := bytecounter.NewReader(&buf)
			r := NewReader(bcr, func(count uint32) error {
				return nil
			})

			for _, entry := range ca.sequence {
				buf2, err := entry.chunk.Marshal()
				require.NoError(t, err)
				buf.Write(buf2)

				if entry.msg != nil {
					msg, err := r.Read()
					require.NoError(t, err)
					require.Equal(t, entry.msg, msg)
				}
			}
		})
	}
}

func TestReaderAcknowledge(t *testing.T) {
	for _, ca := range []string{"standard", "overflow"} {
		t.Run(ca, func(t *testing.T) {
			onAckCalled := make(chan struct{})

			var buf bytes.Buffer
			bcr := bytecounter.NewReader(&buf)
			r := NewReader(bcr, func(count uint32) error {
				close(onAckCalled)
				return nil
			})

			if ca == "overflow" {
				bcr.SetCount(4294967096)
				r.lastAckCount = 4294967096
			}

			r.SetChunkSize(65536)
			r.SetWindowAckSize(100)

			buf2, err := chunk.Chunk0{
				ChunkStreamID:   27,
				Timestamp:       18576,
				Type:            chunk.MessageTypeSetPeerBandwidth,
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
