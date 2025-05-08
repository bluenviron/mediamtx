package message

import (
	"fmt"
	"time"

	"github.com/bluenviron/mediamtx/src/protocols/rtmp/rawmessage"
)

// AudioExCodedFrames is a CodedFrames extended message.
type AudioExCodedFrames struct {
	ChunkStreamID   byte
	DTS             time.Duration
	MessageStreamID uint32
	FourCC          FourCC
	Payload         []byte
}

func (m *AudioExCodedFrames) unmarshal(raw *rawmessage.Message) error {
	if len(raw.Body) < 5 {
		return fmt.Errorf("not enough bytes")
	}

	m.ChunkStreamID = raw.ChunkStreamID
	m.DTS = raw.Timestamp
	m.MessageStreamID = raw.MessageStreamID

	m.FourCC = FourCC(raw.Body[1])<<24 | FourCC(raw.Body[2])<<16 | FourCC(raw.Body[3])<<8 | FourCC(raw.Body[4])
	switch m.FourCC {
	case FourCCOpus, FourCCAC3, FourCCMP4A, FourCCMP3:
	default:
		return fmt.Errorf("unsupported fourCC: %v", m.FourCC)
	}

	m.Payload = raw.Body[5:]

	return nil
}

func (m AudioExCodedFrames) marshalBodySize() int {
	return 5 + len(m.Payload)
}

func (m AudioExCodedFrames) marshal() (*rawmessage.Message, error) {
	body := make([]byte, m.marshalBodySize())

	body[0] = (9 << 4) | byte(AudioExTypeCodedFrames)
	body[1] = uint8(m.FourCC >> 24)
	body[2] = uint8(m.FourCC >> 16)
	body[3] = uint8(m.FourCC >> 8)
	body[4] = uint8(m.FourCC)
	copy(body[5:], m.Payload)

	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Timestamp:       m.DTS,
		Type:            uint8(TypeAudio),
		MessageStreamID: m.MessageStreamID,
		Body:            body,
	}, nil
}
