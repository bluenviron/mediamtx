package message

import (
	"fmt"
	"time"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/rawmessage"
)

// VideoExCodedFrames is a CodedFrames extended message.
type VideoExCodedFrames struct {
	ChunkStreamID   byte
	DTS             time.Duration
	MessageStreamID uint32
	FourCC          FourCC
	PTSDelta        time.Duration
	Payload         []byte
}

func (m *VideoExCodedFrames) unmarshal(raw *rawmessage.Message) error {
	if len(raw.Body) < 5 {
		return fmt.Errorf("not enough bytes")
	}

	m.ChunkStreamID = raw.ChunkStreamID
	m.DTS = raw.Timestamp
	m.MessageStreamID = raw.MessageStreamID

	m.FourCC = FourCC(raw.Body[1])<<24 | FourCC(raw.Body[2])<<16 | FourCC(raw.Body[3])<<8 | FourCC(raw.Body[4])
	switch m.FourCC {
	case FourCCAVC, FourCCHEVC:
		if len(raw.Body) < 8 {
			return fmt.Errorf("not enough bytes")
		}
		m.PTSDelta = time.Duration(uint32(raw.Body[5])<<16|uint32(raw.Body[6])<<8|uint32(raw.Body[7])) * time.Millisecond
		m.Payload = raw.Body[8:]

	case FourCCAV1, FourCCVP9:
		m.Payload = raw.Body[5:]

	default:
		return fmt.Errorf("unsupported fourCC: %v", m.FourCC)
	}

	return nil
}

func (m VideoExCodedFrames) marshalBodySize() int {
	switch m.FourCC {
	case FourCCAVC, FourCCHEVC:
		return 8 + len(m.Payload)
	}
	return 5 + len(m.Payload)
}

func (m VideoExCodedFrames) marshal() (*rawmessage.Message, error) {
	body := make([]byte, m.marshalBodySize())

	body[0] = 0b10000000 | byte(VideoExTypeCodedFrames)
	body[1] = uint8(m.FourCC >> 24)
	body[2] = uint8(m.FourCC >> 16)
	body[3] = uint8(m.FourCC >> 8)
	body[4] = uint8(m.FourCC)

	if m.FourCC == FourCCHEVC {
		tmp := uint32(m.PTSDelta / time.Millisecond)
		body[5] = uint8(tmp >> 16)
		body[6] = uint8(tmp >> 8)
		body[7] = uint8(tmp)
		copy(body[8:], m.Payload)
	} else {
		copy(body[5:], m.Payload)
	}

	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Timestamp:       m.DTS,
		Type:            uint8(TypeVideo),
		MessageStreamID: m.MessageStreamID,
		Body:            body,
	}, nil
}
