package message

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/rawmessage"
)

// ExtendedMPEG2TSSequenceStart is a MPEG2-TS sequence start extended message.
type ExtendedMPEG2TSSequenceStart struct {
	FourCC FourCC
}

func (m *ExtendedMPEG2TSSequenceStart) unmarshal(raw *rawmessage.Message) error {
	if len(raw.Body) != 5 {
		return fmt.Errorf("invalid body size")
	}

	m.FourCC = FourCC(raw.Body[1])<<24 | FourCC(raw.Body[2])<<16 | FourCC(raw.Body[3])<<8 | FourCC(raw.Body[4])

	return fmt.Errorf("ExtendedMPEG2TSSequenceStart is not implemented yet")
}

func (m ExtendedMPEG2TSSequenceStart) marshal() (*rawmessage.Message, error) {
	return nil, fmt.Errorf("TODO")
}
