package message

import (
	"encoding/binary"
	"fmt"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/rawmessage"
)

// MsgUserControlSetBufferLength is a user control message.
type MsgUserControlSetBufferLength struct {
	StreamID     uint32
	BufferLength uint32
}

// Unmarshal implements Message.
func (m *MsgUserControlSetBufferLength) Unmarshal(raw *rawmessage.Message) error {
	if raw.ChunkStreamID != ControlChunkStreamID {
		return fmt.Errorf("unexpected chunk stream ID")
	}

	if len(raw.Body) != 10 {
		return fmt.Errorf("invalid body size")
	}

	m.StreamID = binary.BigEndian.Uint32(raw.Body[2:])
	m.BufferLength = binary.BigEndian.Uint32(raw.Body[6:])

	return nil
}

// Marshal implements Message.
func (m MsgUserControlSetBufferLength) Marshal() (*rawmessage.Message, error) {
	body := make([]byte, 10)
	binary.BigEndian.PutUint16(body, UserControlTypeSetBufferLength)
	binary.BigEndian.PutUint32(body[2:], m.StreamID)
	binary.BigEndian.PutUint32(body[6:], m.BufferLength)

	return &rawmessage.Message{
		ChunkStreamID: ControlChunkStreamID,
		Type:          chunk.MessageTypeUserControl,
		Body:          body,
	}, nil
}
