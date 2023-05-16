package message

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/rtmp/rawmessage"
)

// ExtendedSequenceEnd is a sequence end extended message.
type ExtendedSequenceEnd struct {
	FourCC [4]byte
}

// Unmarshal implements Message.
func (m *ExtendedSequenceEnd) Unmarshal(raw *rawmessage.Message) error {
	if len(raw.Body) != 5 {
		return fmt.Errorf("invalid body size")
	}

	copy(m.FourCC[:], raw.Body[1:5])

	return nil
}

// Marshal implements Message.
func (m ExtendedSequenceEnd) Marshal() (*rawmessage.Message, error) {
	return nil, fmt.Errorf("TODO")
}
