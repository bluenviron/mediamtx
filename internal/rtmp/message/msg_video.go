package message

import (
	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/rawmessage"
)

// MsgVideo is a video message.
type MsgVideo struct {
	ChunkStreamID   byte
	MessageStreamID uint32
	Body            []byte
}

// Unmarshal implements Message.
func (m *MsgVideo) Unmarshal(raw *rawmessage.Message) error {
	m.ChunkStreamID = raw.ChunkStreamID
	m.MessageStreamID = raw.MessageStreamID
	m.Body = raw.Body
	return nil
}

// Marshal implements Message.
func (m MsgVideo) Marshal() (*rawmessage.Message, error) {
	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Type:            chunk.MessageTypeVideo,
		MessageStreamID: m.MessageStreamID,
		Body:            m.Body,
	}, nil
}
