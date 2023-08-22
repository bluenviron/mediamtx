package message

import (
	"github.com/bluenviron/mediamtx/internal/rtmp/rawmessage"
)

// ExtendedSequenceStart is a sequence start extended message.
type ExtendedSequenceStart struct {
	ChunkStreamID   byte
	MessageStreamID uint32
	FourCC          [4]byte
	Config          []byte
}

// Unmarshal implements Message.
func (m *ExtendedSequenceStart) Unmarshal(raw *rawmessage.Message) error {
	m.ChunkStreamID = raw.ChunkStreamID
	m.MessageStreamID = raw.MessageStreamID
	copy(m.FourCC[:], raw.Body[1:5])
	m.Config = raw.Body[5:]

	return nil
}

// Marshal implements Message.
func (m ExtendedSequenceStart) Marshal() (*rawmessage.Message, error) {
	body := make([]byte, 5+len(m.Config))

	body[0] = 0b10000000 | byte(ExtendedTypeSequenceStart)
	copy(body[1:5], m.FourCC[:])
	copy(body[5:], m.Config)

	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Type:            uint8(TypeVideo),
		MessageStreamID: m.MessageStreamID,
		Body:            body,
	}, nil
}
