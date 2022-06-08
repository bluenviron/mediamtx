package message

import (
	"encoding/binary"
	"fmt"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/rawmessage"
)

// MsgAcknowledge is an acknowledgement message.
type MsgAcknowledge struct {
	Value uint32
}

// Unmarshal implements Message.
func (m *MsgAcknowledge) Unmarshal(raw *rawmessage.Message) error {
	if raw.ChunkStreamID != ControlChunkStreamID {
		return fmt.Errorf("unexpected chunk stream ID")
	}

	if len(raw.Body) != 4 {
		return fmt.Errorf("unexpected body size")
	}

	m.Value = binary.BigEndian.Uint32(raw.Body)
	return nil
}

// Marshal implements Message.
func (m *MsgAcknowledge) Marshal() (*rawmessage.Message, error) {
	body := make([]byte, 4)
	binary.BigEndian.PutUint32(body, m.Value)

	return &rawmessage.Message{
		ChunkStreamID: ControlChunkStreamID,
		Type:          chunk.MessageTypeAcknowledge,
		Body:          body,
	}, nil
}
