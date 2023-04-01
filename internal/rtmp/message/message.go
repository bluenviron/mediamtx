// Package message contains a RTMP message reader/writer.
package message

import (
	"github.com/aler9/mediamtx/internal/rtmp/rawmessage"
)

const (
	// ControlChunkStreamID is the stream ID used for control messages.
	ControlChunkStreamID = 2
)

// Message is a message.
type Message interface {
	Unmarshal(*rawmessage.Message) error
	Marshal() (*rawmessage.Message, error)
}
