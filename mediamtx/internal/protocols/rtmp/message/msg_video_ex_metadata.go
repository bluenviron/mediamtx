package message

import (
	"fmt"
	"time"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/amf0"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/rawmessage"
)

// VideoExMetadata is a metadata extended message.
type VideoExMetadata struct {
	ChunkStreamID   byte
	DTS             time.Duration
	MessageStreamID uint32
	FourCC          FourCC
	Payload         amf0.Data
}

func (m *VideoExMetadata) unmarshal(raw *rawmessage.Message) error {
	if len(raw.Body) < 6 {
		return fmt.Errorf("invalid body size")
	}

	m.ChunkStreamID = raw.ChunkStreamID
	m.DTS = raw.Timestamp
	m.MessageStreamID = raw.MessageStreamID

	m.FourCC = FourCC(raw.Body[1])<<24 | FourCC(raw.Body[2])<<16 | FourCC(raw.Body[3])<<8 | FourCC(raw.Body[4])
	switch m.FourCC {
	case FourCCAV1, FourCCVP9, FourCCHEVC:
	default:
		return fmt.Errorf("unsupported fourCC: %v", m.FourCC)
	}

	var err error
	m.Payload, err = amf0.Unmarshal(raw.Body[5:])
	if err != nil {
		return err
	}

	return nil
}

func (m VideoExMetadata) marshalBodySize() (int, error) {
	ms, err := m.Payload.MarshalSize()
	if err != nil {
		return 0, err
	}
	return 5 + ms, nil
}

func (m VideoExMetadata) marshal() (*rawmessage.Message, error) {
	mbs, err := m.marshalBodySize()
	if err != nil {
		return nil, err
	}
	body := make([]byte, mbs)

	body[0] = 0b10000000 | byte(VideoExTypeMetadata)
	body[1] = uint8(m.FourCC >> 24)
	body[2] = uint8(m.FourCC >> 16)
	body[3] = uint8(m.FourCC >> 8)
	body[4] = uint8(m.FourCC)

	_, err = m.Payload.MarshalTo(body[5:])
	if err != nil {
		return nil, err
	}

	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Timestamp:       m.DTS,
		Type:            uint8(TypeVideo),
		MessageStreamID: m.MessageStreamID,
		Body:            body,
	}, nil
}
