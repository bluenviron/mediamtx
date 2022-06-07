package message

import (
	"fmt"

	"github.com/notedit/rtmp/format/flv/flvio"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/rawmessage"
)

// MsgVideo is a video message.
type MsgVideo struct {
	ChunkStreamID   byte
	DTS             uint32
	MessageStreamID uint32
	IsKeyFrame      bool
	H264Type        uint8
	PTSDelta        uint32
	Payload         []byte
}

// Unmarshal implements Message.
func (m *MsgVideo) Unmarshal(raw *rawmessage.Message) error {
	m.ChunkStreamID = raw.ChunkStreamID
	m.DTS = raw.Timestamp
	m.MessageStreamID = raw.MessageStreamID

	if len(raw.Body) < 5 {
		return fmt.Errorf("invalid body size")
	}

	m.IsKeyFrame = (raw.Body[0] >> 4) == flvio.FRAME_KEY

	codec := raw.Body[0] & 0x0F
	if codec != flvio.VIDEO_H264 {
		return fmt.Errorf("unsupported video codec: %d", codec)
	}

	m.H264Type = raw.Body[1]
	m.PTSDelta = uint32(raw.Body[2])<<16 | uint32(raw.Body[3])<<8 | uint32(raw.Body[4])
	m.Payload = raw.Body[5:]

	return nil
}

// Marshal implements Message.
func (m MsgVideo) Marshal() (*rawmessage.Message, error) {
	body := make([]byte, 5+len(m.Payload))

	if m.IsKeyFrame {
		body[0] = flvio.FRAME_KEY << 4
	} else {
		body[0] = flvio.FRAME_INTER << 4
	}
	body[0] |= flvio.VIDEO_H264
	body[1] = m.H264Type
	body[2] = uint8(m.PTSDelta >> 16)
	body[3] = uint8(m.PTSDelta >> 8)
	body[4] = uint8(m.PTSDelta)
	copy(body[5:], m.Payload)

	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Timestamp:       m.DTS,
		Type:            chunk.MessageTypeVideo,
		MessageStreamID: m.MessageStreamID,
		Body:            body,
	}, nil
}
