package message

import (
	"encoding/binary"
	"fmt"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/rawmessage"
)

// MsgUserControl is a user control message.
type MsgUserControl struct {
	Type    uint16
	Payload []byte
}

// Unmarshal implements Message.
func (m *MsgUserControl) Unmarshal(raw *rawmessage.Message) error {
	if raw.ChunkStreamID != ControlChunkStreamID {
		return fmt.Errorf("unexpected chunk stream ID")
	}

	if len(raw.Body) < 2 {
		return fmt.Errorf("unexpected body size")
	}

	m.Type = binary.BigEndian.Uint16(raw.Body)
	m.Payload = raw.Body[2:]

	return nil
}

// Marshal implements Message.
func (m MsgUserControl) Marshal() (*rawmessage.Message, error) {
	body := make([]byte, 2+len(m.Payload))
	binary.BigEndian.PutUint16(body, m.Type)
	copy(body[2:], m.Payload)

	return &rawmessage.Message{
		ChunkStreamID: ControlChunkStreamID,
		Type:          chunk.MessageTypeUserControl,
		Body:          body,
	}, nil
}
