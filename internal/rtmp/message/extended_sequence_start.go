package message

import (
	"fmt"

	"github.com/aler9/mediamtx/internal/rtmp/rawmessage"
)

// ExtendedSequenceStart is a sequence start extended message.
type ExtendedSequenceStart struct {
	FourCC [4]byte
	Config []byte
}

// Unmarshal implements Message.
func (m *ExtendedSequenceStart) Unmarshal(raw *rawmessage.Message) error {
	copy(m.FourCC[:], raw.Body[1:5])
	m.Config = raw.Body[5:]

	return nil
}

// Marshal implements Message.
func (m ExtendedSequenceStart) Marshal() (*rawmessage.Message, error) {
	return nil, fmt.Errorf("TODO")
}
