package message //nolint:dupl

import (
	"fmt"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/rawmessage"
)

// MsgUserControlStreamEOF is a user control message.
type MsgUserControlStreamEOF struct {
	StreamID uint32
}

// Unmarshal implements Message.
func (m *MsgUserControlStreamEOF) Unmarshal(raw *rawmessage.Message) error {
	if raw.ChunkStreamID != ControlChunkStreamID {
		return fmt.Errorf("unexpected chunk stream ID")
	}

	if len(raw.Body) != 6 {
		return fmt.Errorf("invalid body size")
	}

	m.StreamID = uint32(raw.Body[2])<<24 | uint32(raw.Body[3])<<16 | uint32(raw.Body[4])<<8 | uint32(raw.Body[5])

	return nil
}

// Marshal implements Message.
func (m MsgUserControlStreamEOF) Marshal() (*rawmessage.Message, error) {
	buf := make([]byte, 6)

	buf[0] = byte(UserControlTypeStreamEOF >> 8)
	buf[1] = byte(UserControlTypeStreamEOF)
	buf[2] = byte(m.StreamID >> 24)
	buf[3] = byte(m.StreamID >> 16)
	buf[4] = byte(m.StreamID >> 8)
	buf[5] = byte(m.StreamID)

	return &rawmessage.Message{
		ChunkStreamID: ControlChunkStreamID,
		Type:          chunk.MessageTypeUserControl,
		Body:          buf,
	}, nil
}
