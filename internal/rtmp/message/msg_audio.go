package message

import (
	"fmt"

	"github.com/notedit/rtmp/format/flv/flvio"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/rawmessage"
)

// MsgAudio is an audio message.
type MsgAudio struct {
	ChunkStreamID   byte
	DTS             uint32
	MessageStreamID uint32
	Rate            uint8
	Depth           uint8
	Channels        uint8
	AACType         uint8
	Payload         []byte
}

// Unmarshal implements Message.
func (m *MsgAudio) Unmarshal(raw *rawmessage.Message) error {
	m.ChunkStreamID = raw.ChunkStreamID
	m.DTS = raw.Timestamp
	m.MessageStreamID = raw.MessageStreamID

	if len(raw.Body) < 2 {
		return fmt.Errorf("invalid body size")
	}

	codec := raw.Body[0] >> 4
	if codec != flvio.SOUND_AAC {
		return fmt.Errorf("unsupported audio codec: %d", codec)
	}

	m.Rate = (raw.Body[0] >> 2) & 0x03
	m.Depth = (raw.Body[0] >> 1) & 0x01
	m.Channels = raw.Body[0] & 0x01
	m.AACType = raw.Body[1]
	m.Payload = raw.Body[2:]

	return nil
}

// Marshal implements Message.
func (m MsgAudio) Marshal() (*rawmessage.Message, error) {
	body := make([]byte, 2+len(m.Payload))

	body[0] = flvio.SOUND_AAC<<4 | m.Rate<<2 | m.Depth<<1 | m.Channels
	body[1] = m.AACType

	copy(body[2:], m.Payload)

	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Timestamp:       m.DTS,
		Type:            chunk.MessageTypeAudio,
		MessageStreamID: m.MessageStreamID,
		Body:            body,
	}, nil
}
