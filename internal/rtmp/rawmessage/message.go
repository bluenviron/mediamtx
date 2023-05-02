// Package rawmessage contains a RTMP raw message reader/writer.
package rawmessage

import (
	"time"
)

// Message is a raw message.
type Message struct {
	ChunkStreamID   byte
	Timestamp       time.Duration
	Type            uint8
	MessageStreamID uint32
	Body            []byte
}
