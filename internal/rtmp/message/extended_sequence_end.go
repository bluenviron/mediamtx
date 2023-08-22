package message

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/rtmp/rawmessage"
)

// ExtendedSequenceEnd is a sequence end extended message.
type ExtendedSequenceEnd struct {
	FourCC FourCC
}

// Unmarshal implements Message.
func (m *ExtendedSequenceEnd) Unmarshal(raw *rawmessage.Message) error {
	if len(raw.Body) != 5 {
		return fmt.Errorf("invalid body size")
	}

	m.FourCC = FourCC(raw.Body[1])<<24 | FourCC(raw.Body[2])<<16 | FourCC(raw.Body[3])<<8 | FourCC(raw.Body[4])

	return nil
}

// Marshal implements Message.
func (m ExtendedSequenceEnd) Marshal() (*rawmessage.Message, error) {
	return nil, fmt.Errorf("TODO")
}
