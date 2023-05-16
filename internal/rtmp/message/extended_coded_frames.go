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
	FourCC          [4]byte
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
	copy(m.FourCC[:], raw.Body[1:5])

	if m.FourCC == FourCCHEVC {
		m.PTSDelta = time.Duration(uint32(raw.Body[5])<<16|uint32(raw.Body[6])<<8|uint32(raw.Body[7])) * time.Millisecond
		m.Payload = raw.Body[8:]
	} else {
		m.Payload = raw.Body[5:]
	}

	return nil
}

// Marshal implements Message.
func (m ExtendedCodedFrames) Marshal() (*rawmessage.Message, error) {
	var l int
	if m.FourCC == FourCCHEVC {
		l = 8 + len(m.Payload)
	} else {
		l = 5 + len(m.Payload)
	}
	body := make([]byte, l)

	body[0] = 0b10000000 | byte(ExtendedTypeCodedFrames)
	copy(body[1:5], m.FourCC[:])

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
