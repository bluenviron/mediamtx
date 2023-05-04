package message

import (
	"time"

	"github.com/aler9/mediamtx/internal/rtmp/rawmessage"
)

// ExtendedFramesX is a FramesX extended message.
type ExtendedFramesX struct {
	ChunkStreamID   byte
	DTS             time.Duration
	MessageStreamID uint32
	FourCC          [4]byte
	Payload         []byte
}

// Unmarshal implements Message.
func (m *ExtendedFramesX) Unmarshal(raw *rawmessage.Message) error {
	m.ChunkStreamID = raw.ChunkStreamID
	m.DTS = raw.Timestamp
	m.MessageStreamID = raw.MessageStreamID
	copy(m.FourCC[:], raw.Body[1:5])
	m.Payload = raw.Body[5:]

	return nil
}

// Marshal implements Message.
func (m ExtendedFramesX) Marshal() (*rawmessage.Message, error) {
	body := make([]byte, 5+len(m.Payload))

	body[0] = 0b10000000 | byte(ExtendedTypeFramesX)
	copy(body[1:5], m.FourCC[:])
	copy(body[5:], m.Payload)

	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Timestamp:       m.DTS,
		Type:            uint8(TypeVideo),
		MessageStreamID: m.MessageStreamID,
		Body:            body,
	}, nil
}
