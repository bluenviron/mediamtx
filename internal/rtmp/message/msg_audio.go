package message

import (
	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/rawmessage"
)

// MsgAudio is an audio message.
type MsgAudio struct {
	ChunkStreamID   byte
	MessageStreamID uint32
	Body            []byte
}

// Unmarshal implements Message.
func (m *MsgAudio) Unmarshal(raw *rawmessage.Message) error {
	m.ChunkStreamID = raw.ChunkStreamID
	m.MessageStreamID = raw.MessageStreamID
	m.Body = raw.Body
	return nil
}

// Marshal implements Message.
func (m MsgAudio) Marshal() (*rawmessage.Message, error) {
	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Type:            chunk.MessageTypeAudio,
		MessageStreamID: m.MessageStreamID,
		Body:            m.Body,
	}, nil
}
