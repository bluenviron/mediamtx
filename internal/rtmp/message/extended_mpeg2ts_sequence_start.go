package message

import (
	"fmt"

	"github.com/aler9/mediamtx/internal/rtmp/rawmessage"
)

// ExtendedMPEG2TSSequenceStart is a MPEG2-TS sequence start extended message.
type ExtendedMPEG2TSSequenceStart struct {
	FourCC [4]byte
}

// Unmarshal implements Message.
func (m *ExtendedMPEG2TSSequenceStart) Unmarshal(raw *rawmessage.Message) error {
	copy(m.FourCC[:], raw.Body[1:5])

	return fmt.Errorf("ExtendedMPEG2TSSequenceStart is not implemented yet")
}

// Marshal implements Message.
func (m ExtendedMPEG2TSSequenceStart) Marshal() (*rawmessage.Message, error) {
	return nil, fmt.Errorf("TODO")
}
