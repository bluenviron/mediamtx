package message

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/rawmessage"
)

// AudioExSequenceEnd is a sequence end extended message.
type AudioExSequenceEnd struct {
	ChunkStreamID   byte
	MessageStreamID uint32
	FourCC          FourCC
}

func (m *AudioExSequenceEnd) unmarshal(raw *rawmessage.Message) error {
	if len(raw.Body) != 5 {
		return fmt.Errorf("not enough bytes")
	}

	m.ChunkStreamID = raw.ChunkStreamID
	m.MessageStreamID = raw.MessageStreamID

	m.FourCC = FourCC(raw.Body[1])<<24 | FourCC(raw.Body[2])<<16 | FourCC(raw.Body[3])<<8 | FourCC(raw.Body[4])
	switch m.FourCC {
	case FourCCOpus, FourCCAC3, FourCCMP4A, FourCCMP3:
	default:
		return fmt.Errorf("unsupported fourCC: %v", m.FourCC)
	}

	return nil
}

func (m AudioExSequenceEnd) marshal() (*rawmessage.Message, error) {
	body := make([]byte, 5)

	body[0] = (9 << 4) | byte(AudioExTypeSequenceEnd)
	body[1] = uint8(m.FourCC >> 24)
	body[2] = uint8(m.FourCC >> 16)
	body[3] = uint8(m.FourCC >> 8)
	body[4] = uint8(m.FourCC)

	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Type:            uint8(TypeAudio),
		MessageStreamID: m.MessageStreamID,
		Body:            body,
	}, nil
}
