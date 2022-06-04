package message

import (
	"github.com/aler9/rtsp-simple-server/internal/rtmp/rawmessage"
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
