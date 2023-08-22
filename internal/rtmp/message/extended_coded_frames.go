package message

import (
	"fmt"
	"time"

	"github.com/bluenviron/mediamtx/internal/rtmp/rawmessage"
)

// ExtendedCodedFrames is a CodedFrames extended message.
type ExtendedCodedFrames struct {
	ChunkStreamID   byte
	DTS             time.Duration
	MessageStreamID uint32
	FourCC          FourCC
	PTSDelta        time.Duration
	Payload         []byte
}

// Unmarshal implements Message.
func (m *ExtendedCodedFrames) Unmarshal(raw *rawmessage.Message) error {
	if len(raw.Body) < 8 {
		return fmt.Errorf("not enough bytes")
	}

	m.ChunkStreamID = raw.ChunkStreamID
	m.DTS = raw.Timestamp
	m.MessageStreamID = raw.MessageStreamID
	m.FourCC = FourCC(raw.Body[1])<<24 | FourCC(raw.Body[2])<<16 | FourCC(raw.Body[3])<<8 | FourCC(raw.Body[4])

	if m.FourCC == FourCCHEVC {
		m.PTSDelta = time.Duration(uint32(raw.Body[5])<<16|uint32(raw.Body[6])<<8|uint32(raw.Body[7])) * time.Millisecond
		m.Payload = raw.Body[8:]
	} else {
		m.Payload = raw.Body[5:]
	}

	return nil
}

func (m ExtendedCodedFrames) marshalBodySize() int {
	var l int
	if m.FourCC == FourCCHEVC {
		l = 8 + len(m.Payload)
	} else {
		l = 5 + len(m.Payload)
	}
	return l
}

// Marshal implements Message.
func (m ExtendedCodedFrames) Marshal() (*rawmessage.Message, error) {
	body := make([]byte, m.marshalBodySize())

	body[0] = 0b10000000 | byte(ExtendedTypeCodedFrames)
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
