// Package rawmessage contains a RTMP raw message reader/writer.
package rawmessage

import (
	"time"

	"github.com/aler9/mediamtx/internal/rtmp/chunk"
)

// Message is a raw message.
type Message struct {
	ChunkStreamID   byte
	Timestamp       time.Duration
	Type            chunk.MessageType
	MessageStreamID uint32
	Body            []byte
}
