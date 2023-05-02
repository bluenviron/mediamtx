package message

import (
	"fmt"
	"time"

	"github.com/aler9/mediamtx/internal/rtmp/chunk"
	"github.com/aler9/mediamtx/internal/rtmp/rawmessage"
)

const (
	// MsgAudioChunkStreamID is the chunk stream ID that is usually used to send MsgAudio{}
	MsgAudioChunkStreamID = 4
)

// supported audio codecs
const (
	CodecMPEG2Audio = 2
	CodecMPEG4Audio = 10
)

// MsgAudioAACType is the AAC type of a MsgAudio.
type MsgAudioAACType uint8

// MsgAudioAACType values.
const (
	MsgAudioAACTypeConfig MsgAudioAACType = 0
	MsgAudioAACTypeAU     MsgAudioAACType = 1
)

// MsgAudio is an audio message.
type MsgAudio struct {
	ChunkStreamID   byte
	DTS             time.Duration
	MessageStreamID uint32
	Codec           uint8
	Rate            uint8
	Depth           uint8
	Channels        uint8
	AACType         MsgAudioAACType // only for CodecMPEG4Audio
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

	m.Codec = raw.Body[0] >> 4
	switch m.Codec {
	case CodecMPEG2Audio, CodecMPEG4Audio:
	default:
		return fmt.Errorf("unsupported audio codec: %d", m.Codec)
	}

	m.Rate = (raw.Body[0] >> 2) & 0x03
	m.Depth = (raw.Body[0] >> 1) & 0x01
	m.Channels = raw.Body[0] & 0x01

	if m.Codec == CodecMPEG2Audio {
		m.Payload = raw.Body[1:]
	} else {
		m.AACType = MsgAudioAACType(raw.Body[1])
		switch m.AACType {
		case MsgAudioAACTypeConfig, MsgAudioAACTypeAU:
		default:
			return fmt.Errorf("unsupported audio message type: %d", m.AACType)
		}

		m.Payload = raw.Body[2:]
	}

	return nil
}

// Marshal implements Message.
func (m MsgAudio) Marshal() (*rawmessage.Message, error) {
	var l int
	if m.Codec == CodecMPEG2Audio {
		l = 1 + len(m.Payload)
	} else {
		l = 2 + len(m.Payload)
	}
	body := make([]byte, l)

	body[0] = m.Codec<<4 | m.Rate<<2 | m.Depth<<1 | m.Channels

	if m.Codec == CodecMPEG2Audio {
		copy(body[1:], m.Payload)
	} else {
		body[1] = uint8(m.AACType)
		copy(body[2:], m.Payload)
	}

	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Timestamp:       m.DTS,
		Type:            chunk.MessageTypeAudio,
		MessageStreamID: m.MessageStreamID,
		Body:            body,
	}, nil
}
