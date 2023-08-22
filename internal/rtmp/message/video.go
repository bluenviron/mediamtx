package message

import (
	"fmt"
	"time"

	"github.com/notedit/rtmp/format/flv/flvio"

	"github.com/bluenviron/mediamtx/internal/rtmp/rawmessage"
)

const (
	// VideoChunkStreamID is the chunk stream ID that is usually used to send Video{}
	VideoChunkStreamID = 6
)

// supported video codecs
const (
	CodecH264 = 7
)

// VideoType is the type of a video message.
type VideoType uint8

// VideoType values.
const (
	VideoTypeConfig VideoType = 0
	VideoTypeAU     VideoType = 1
	VideoTypeEOS    VideoType = 2
)

// Video is a video message.
type Video struct {
	ChunkStreamID   byte
	DTS             time.Duration
	MessageStreamID uint32
	Codec           uint8
	IsKeyFrame      bool
	Type            VideoType
	PTSDelta        time.Duration
	Payload         []byte
}

// Unmarshal implements Message.
func (m *Video) Unmarshal(raw *rawmessage.Message) error {
	m.ChunkStreamID = raw.ChunkStreamID
	m.DTS = raw.Timestamp
	m.MessageStreamID = raw.MessageStreamID

	if len(raw.Body) < 5 {
		return fmt.Errorf("invalid body size")
	}

	m.IsKeyFrame = (raw.Body[0] >> 4) == flvio.FRAME_KEY

	m.Codec = raw.Body[0] & 0x0F
	switch m.Codec {
	case CodecH264:
	default:
		return fmt.Errorf("unsupported video codec: %d", m.Codec)
	}

	m.Type = VideoType(raw.Body[1])
	switch m.Type {
	case VideoTypeConfig, VideoTypeAU, VideoTypeEOS:
	default:
		return fmt.Errorf("unsupported video message type: %d", m.Type)
	}

	m.PTSDelta = time.Duration(uint32(raw.Body[2])<<16|uint32(raw.Body[3])<<8|uint32(raw.Body[4])) * time.Millisecond

	m.Payload = raw.Body[5:]

	return nil
}

func (m Video) marshalBodySize() int {
	return 5 + len(m.Payload)
}

// Marshal implements Message.
func (m Video) Marshal() (*rawmessage.Message, error) {
	body := make([]byte, m.marshalBodySize())

	if m.IsKeyFrame {
		body[0] = flvio.FRAME_KEY << 4
	} else {
		body[0] = flvio.FRAME_INTER << 4
	}
	body[0] |= m.Codec
	body[1] = uint8(m.Type)

	tmp := uint32(m.PTSDelta / time.Millisecond)
	body[2] = uint8(tmp >> 16)
	body[3] = uint8(tmp >> 8)
	body[4] = uint8(tmp)

	copy(body[5:], m.Payload)

	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Timestamp:       m.DTS,
		Type:            uint8(TypeVideo),
		MessageStreamID: m.MessageStreamID,
		Body:            body,
	}, nil
}
