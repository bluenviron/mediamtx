package message

import (
	"fmt"
	"time"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/rawmessage"
)

const (
	// AudioChunkStreamID is the chunk stream ID that is usually used to send Audio{}
	AudioChunkStreamID = 4
)

// audio codecs
const (
	CodecMPEG1Audio = 2
	CodecLPCM       = 3
	CodecPCMA       = 7
	CodecPCMU       = 8
	CodecMPEG4Audio = 10
)

// audio rates
const (
	Rate5512  = 0
	Rate11025 = 1
	Rate22050 = 2
	Rate44100 = 3
)

// audio depths
const (
	Depth8  = 0
	Depth16 = 1
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
	IsStereo        bool
	AACType         AudioAACType // only for CodecMPEG4Audio
	Payload         []byte
}

func (m *Audio) unmarshal(raw *rawmessage.Message) error {
	m.ChunkStreamID = raw.ChunkStreamID
	m.DTS = raw.Timestamp
	m.MessageStreamID = raw.MessageStreamID

	if len(raw.Body) < 2 {
		return fmt.Errorf("invalid body size")
	}

	m.Codec = raw.Body[0] >> 4
	switch m.Codec {
	case CodecMPEG4Audio, CodecMPEG1Audio, CodecPCMA, CodecPCMU, CodecLPCM:
	default:
		return fmt.Errorf("unsupported audio codec: %d", m.Codec)
	}

	m.Rate = (raw.Body[0] >> 2) & 0x03
	m.Depth = (raw.Body[0] >> 1) & 0x01

	if (raw.Body[0] & 0x01) != 0 {
		m.IsStereo = true
	}

	if m.Codec == CodecMPEG4Audio {
		m.AACType = AudioAACType(raw.Body[1])
		switch m.AACType {
		case AudioAACTypeConfig, AudioAACTypeAU:
		default:
			return fmt.Errorf("unsupported audio message type: %d", m.AACType)
		}

		m.Payload = raw.Body[2:]
	} else {
		m.Payload = raw.Body[1:]
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

func (m Audio) marshal() (*rawmessage.Message, error) {
	body := make([]byte, m.marshalBodySize())

	body[0] = m.Codec<<4 | m.Rate<<2 | m.Depth<<1

	if m.IsStereo {
		body[0] |= 1
	}

	if m.Codec == CodecMPEG4Audio {
		body[1] = uint8(m.AACType)
		copy(body[2:], m.Payload)
	} else {
		copy(body[1:], m.Payload)
	}

	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Timestamp:       m.DTS,
		Type:            uint8(TypeAudio),
		MessageStreamID: m.MessageStreamID,
		Body:            body,
	}, nil
}
