package message //nolint:dupl

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/rawmessage"
)

// UserControlSetBufferLength is a user control message.
type UserControlSetBufferLength struct {
	StreamID     uint32
	BufferLength uint32
}

func (m *UserControlSetBufferLength) unmarshal(raw *rawmessage.Message) error {
	if raw.ChunkStreamID != ControlChunkStreamID {
		return fmt.Errorf("unexpected chunk stream ID")
	}

	if len(raw.Body) != 10 {
		return fmt.Errorf("invalid body size")
	}

	m.StreamID = uint32(raw.Body[2])<<24 | uint32(raw.Body[3])<<16 | uint32(raw.Body[4])<<8 | uint32(raw.Body[5])
	m.BufferLength = uint32(raw.Body[6])<<24 | uint32(raw.Body[7])<<16 | uint32(raw.Body[8])<<8 | uint32(raw.Body[9])

	return nil
}

func (m UserControlSetBufferLength) marshal() (*rawmessage.Message, error) {
	buf := make([]byte, 10)

	buf[0] = byte(UserControlTypeSetBufferLength >> 8)
	buf[1] = byte(UserControlTypeSetBufferLength)
	buf[2] = byte(m.StreamID >> 24)
	buf[3] = byte(m.StreamID >> 16)
	buf[4] = byte(m.StreamID >> 8)
	buf[5] = byte(m.StreamID)
	buf[6] = byte(m.BufferLength >> 24)
	buf[7] = byte(m.BufferLength >> 16)
	buf[8] = byte(m.BufferLength >> 8)
	buf[9] = byte(m.BufferLength)

	return &rawmessage.Message{
		ChunkStreamID: ControlChunkStreamID,
		Type:          uint8(TypeUserControl),
		Body:          buf,
	}, nil
}
