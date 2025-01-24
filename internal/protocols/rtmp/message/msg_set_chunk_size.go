package message //nolint:dupl

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/rawmessage"
)

// SetChunkSize is a set chunk size message.
type SetChunkSize struct {
	Value uint32
}

func (m *SetChunkSize) unmarshal(raw *rawmessage.Message) error {
	if raw.ChunkStreamID != ControlChunkStreamID {
		return fmt.Errorf("unexpected chunk stream ID")
	}

	if len(raw.Body) != 4 {
		return fmt.Errorf("invalid body size")
	}

	m.Value = uint32(raw.Body[0])<<24 | uint32(raw.Body[1])<<16 | uint32(raw.Body[2])<<8 | uint32(raw.Body[3])

	return nil
}

func (m *SetChunkSize) marshal() (*rawmessage.Message, error) {
	buf := make([]byte, 4)

	buf[0] = byte(m.Value >> 24)
	buf[1] = byte(m.Value >> 16)
	buf[2] = byte(m.Value >> 8)
	buf[3] = byte(m.Value)

	return &rawmessage.Message{
		ChunkStreamID: ControlChunkStreamID,
		Type:          uint8(TypeSetChunkSize),
		Body:          buf,
	}, nil
}
