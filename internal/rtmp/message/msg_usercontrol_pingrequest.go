package message

import (
	"encoding/binary"
	"fmt"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/rawmessage"
)

// MsgUserControlPingRequest is a user control message.
type MsgUserControlPingRequest struct {
	ServerTime uint32
}

// Unmarshal implements Message.
func (m *MsgUserControlPingRequest) Unmarshal(raw *rawmessage.Message) error {
	if raw.ChunkStreamID != ControlChunkStreamID {
		return fmt.Errorf("unexpected chunk stream ID")
	}

	if len(raw.Body) != 6 {
		return fmt.Errorf("invalid body size")
	}

	m.ServerTime = binary.BigEndian.Uint32(raw.Body[2:])

	return nil
}

// Marshal implements Message.
func (m MsgUserControlPingRequest) Marshal() (*rawmessage.Message, error) {
	body := make([]byte, 6)
	binary.BigEndian.PutUint16(body, UserControlTypePingRequest)
	binary.BigEndian.PutUint32(body[2:], m.ServerTime)

	return &rawmessage.Message{
		ChunkStreamID: ControlChunkStreamID,
		Type:          chunk.MessageTypeUserControl,
		Body:          body,
	}, nil
}
