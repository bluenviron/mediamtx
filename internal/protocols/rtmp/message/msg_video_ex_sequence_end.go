package message

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/rawmessage"
)

// VideoExSequenceEnd is a sequence end extended message.
type VideoExSequenceEnd struct {
	ChunkStreamID   byte
	MessageStreamID uint32
	FourCC          FourCC
}

func (m *VideoExSequenceEnd) unmarshal(raw *rawmessage.Message) error {
	if len(raw.Body) != 5 {
		return fmt.Errorf("not enough bytes")
	}

	m.ChunkStreamID = raw.ChunkStreamID
	m.MessageStreamID = raw.MessageStreamID

	m.FourCC = FourCC(raw.Body[1])<<24 | FourCC(raw.Body[2])<<16 | FourCC(raw.Body[3])<<8 | FourCC(raw.Body[4])
	switch m.FourCC {
	case FourCCAV1, FourCCVP9, FourCCHEVC, FourCCAVC:
	default:
		return fmt.Errorf("unsupported fourCC: %v", m.FourCC)
	}

	return nil
}

func (m VideoExSequenceEnd) marshalBodySize() int {
	return 5
}

func (m VideoExSequenceEnd) marshal() (*rawmessage.Message, error) {
	body := make([]byte, m.marshalBodySize())

	body[0] = 0b10000000 | byte(VideoExTypeSequenceEnd)
	body[1] = uint8(m.FourCC >> 24)
	body[2] = uint8(m.FourCC >> 16)
	body[3] = uint8(m.FourCC >> 8)
	body[4] = uint8(m.FourCC)

	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Type:            uint8(TypeVideo),
		MessageStreamID: m.MessageStreamID,
		Body:            body,
	}, nil
}
