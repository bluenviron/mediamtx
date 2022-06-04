package message

import (
	"encoding/binary"
	"fmt"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/rawmessage"
)

// MsgSetPeerBandwidth is a set peer bandwidth message.
type MsgSetPeerBandwidth struct {
	Value uint32
	Type  byte
}

// Unmarshal implements Message.
func (m *MsgSetPeerBandwidth) Unmarshal(raw *rawmessage.Message) error {
	if raw.ChunkStreamID != ControlChunkStreamID {
		return fmt.Errorf("unexpected chunk stream ID")
	}

	if len(raw.Body) != 5 {
		return fmt.Errorf("unexpected body size")
	}

	m.Value = binary.BigEndian.Uint32(raw.Body)
	m.Type = raw.Body[4]
	return nil
}

// Marshal implements Message.
func (m *MsgSetPeerBandwidth) Marshal() (*rawmessage.Message, error) {
	body := make([]byte, 5)
	binary.BigEndian.PutUint32(body, m.Value)
	body[4] = m.Type

	return &rawmessage.Message{
		ChunkStreamID: ControlChunkStreamID,
		Type:          chunk.MessageTypeSetChunkSize,
		Body:          body,
	}, nil
}
