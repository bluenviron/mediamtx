package message

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/rtmp/rawmessage"
)

// ExtendedSequenceStart is a sequence start extended message.
type ExtendedSequenceStart struct {
	ChunkStreamID   byte
	MessageStreamID uint32
	FourCC          FourCC
	Config          []byte
}

// Unmarshal implements Message.
func (m *ExtendedSequenceStart) Unmarshal(raw *rawmessage.Message) error {
	if len(raw.Body) < 6 {
		return fmt.Errorf("not enough bytes")
	}

	m.ChunkStreamID = raw.ChunkStreamID
	m.MessageStreamID = raw.MessageStreamID
	m.FourCC = FourCC(raw.Body[1])<<24 | FourCC(raw.Body[2])<<16 | FourCC(raw.Body[3])<<8 | FourCC(raw.Body[4])
	m.Config = raw.Body[5:]

	return nil
}

func (m ExtendedSequenceStart) marshalBodySize() int {
	return 5 + len(m.Config)
}

// Marshal implements Message.
func (m ExtendedSequenceStart) Marshal() (*rawmessage.Message, error) {
	body := make([]byte, m.marshalBodySize())

	body[0] = 0b10000000 | byte(ExtendedTypeSequenceStart)
	body[1] = uint8(m.FourCC >> 24)
	body[2] = uint8(m.FourCC >> 16)
	body[3] = uint8(m.FourCC >> 8)
	body[4] = uint8(m.FourCC)
	copy(body[5:], m.Config)

	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Type:            uint8(TypeVideo),
		MessageStreamID: m.MessageStreamID,
		Body:            body,
	}, nil
}
