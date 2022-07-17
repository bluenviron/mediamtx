package message //nolint:dupl

import (
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

	m.Value = uint32(raw.Body[0])<<24 | uint32(raw.Body[1])<<16 | uint32(raw.Body[2])<<8 | uint32(raw.Body[3])
	m.Type = raw.Body[4]

	return nil
}

// Marshal implements Message.
func (m *MsgSetPeerBandwidth) Marshal() (*rawmessage.Message, error) {
	buf := make([]byte, 5)

	buf[0] = byte(m.Value >> 24)
	buf[1] = byte(m.Value >> 16)
	buf[2] = byte(m.Value >> 8)
	buf[3] = byte(m.Value)
	buf[4] = m.Type

	return &rawmessage.Message{
		ChunkStreamID: ControlChunkStreamID,
		Type:          chunk.MessageTypeSetPeerBandwidth,
		Body:          buf,
	}, nil
}
