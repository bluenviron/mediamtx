package message

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/rtmp/rawmessage"
)

// ExtendedMetadata is a metadata extended message.
type ExtendedMetadata struct {
	FourCC [4]byte
}

// Unmarshal implements Message.
func (m *ExtendedMetadata) Unmarshal(raw *rawmessage.Message) error {
	copy(m.FourCC[:], raw.Body[1:5])

	return fmt.Errorf("ExtendedMetadata is not implemented yet")
}

// Marshal implements Message.
func (m ExtendedMetadata) Marshal() (*rawmessage.Message, error) {
	return nil, fmt.Errorf("TODO")
}
