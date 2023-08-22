package message

import (
	"fmt"
	"time"

	"github.com/bluenviron/mediamtx/internal/rtmp/rawmessage"
)

const (
	// AudioChunkStreamID is the chunk stream ID that is usually used to send Audio{}
	AudioChunkStreamID = 4
)

// supported audio codecs
const (
	CodecMPEG1Audio = 2
	CodecMPEG4Audio = 10
)

// AudioAACType is the AAC type of a Audio.
type AudioAACType uint8

// AudioAACType values.
const (
	AudioAACTypeConfig AudioAACType = 0
	AudioAACTypeAU     AudioAACType = 1
)

// Audio is an audio message.
type Audio struct {
	ChunkStreamID   byte
	DTS             time.Duration
	MessageStreamID uint32
	Codec           uint8
	Rate            uint8
	Depth           uint8
	Channels        uint8
	AACType         AudioAACType // only for CodecMPEG4Audio
	Payload         []byte
}

// Unmarshal implements Message.
func (m *Audio) Unmarshal(raw *rawmessage.Message) error {
	m.ChunkStreamID = raw.ChunkStreamID
	m.DTS = raw.Timestamp
	m.MessageStreamID = raw.MessageStreamID

	if len(raw.Body) < 2 {
		return fmt.Errorf("invalid body size")
	}

	m.Codec = raw.Body[0] >> 4
	switch m.Codec {
	case CodecMPEG1Audio, CodecMPEG4Audio:
	default:
		return fmt.Errorf("unsupported audio codec: %d", m.Codec)
	}

	m.Rate = (raw.Body[0] >> 2) & 0x03
	m.Depth = (raw.Body[0] >> 1) & 0x01
	m.Channels = raw.Body[0] & 0x01

	if m.Codec == CodecMPEG1Audio {
		m.Payload = raw.Body[1:]
	} else {
		m.AACType = AudioAACType(raw.Body[1])
		switch m.AACType {
		case AudioAACTypeConfig, AudioAACTypeAU:
		default:
			return fmt.Errorf("unsupported audio message type: %d", m.AACType)
		}

		m.Payload = raw.Body[2:]
	}

	return nil
}

func (m Audio) marshalBodySize() int {
	var l int
	if m.Codec == CodecMPEG1Audio {
		l = 1 + len(m.Payload)
	} else {
		l = 2 + len(m.Payload)
	}
	return l
}

// Marshal implements Message.
func (m Audio) Marshal() (*rawmessage.Message, error) {
	body := make([]byte, m.marshalBodySize())

	body[0] = m.Codec<<4 | m.Rate<<2 | m.Depth<<1 | m.Channels

	if m.Codec == CodecMPEG1Audio {
		copy(body[1:], m.Payload)
	} else {
		body[1] = uint8(m.AACType)
		copy(body[2:], m.Payload)
	}

	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Timestamp:       m.DTS,
		Type:            uint8(TypeAudio),
		MessageStreamID: m.MessageStreamID,
		Body:            body,
	}, nil
}
