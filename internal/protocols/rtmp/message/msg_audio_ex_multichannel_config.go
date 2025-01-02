package message

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/rawmessage"
)

// AudioExChannelOrder is an audio channel order.
type AudioExChannelOrder uint8

// audio channel orders.
const (
	AudioExChannelOrderUnspecified AudioExChannelOrder = 0
	AudioExChannelOrderNative      AudioExChannelOrder = 1
	AudioExChannelOrderCustom      AudioExChannelOrder = 2
)

// AudioExMultichannelConfig is a multichannel config extended message.
type AudioExMultichannelConfig struct {
	ChunkStreamID       byte
	MessageStreamID     uint32
	FourCC              FourCC
	AudioChannelOrder   AudioExChannelOrder
	ChannelCount        uint8
	AudioChannelMapping uint8  // if AudioChannelOrder == AudioExChannelOrderCustom
	AudioChannelFlags   uint32 // if AudioChannelOrder == AudioExChannelOrderNative
}

func (m *AudioExMultichannelConfig) unmarshal(raw *rawmessage.Message) error {
	if len(raw.Body) < 7 {
		return fmt.Errorf("not enough bytes")
	}

	m.ChunkStreamID = raw.ChunkStreamID
	m.MessageStreamID = raw.MessageStreamID

	m.FourCC = FourCC(raw.Body[1])<<24 | FourCC(raw.Body[2])<<16 | FourCC(raw.Body[3])<<8 | FourCC(raw.Body[4])
	switch m.FourCC {
	case FourCCOpus, FourCCAC3, FourCCMP4A, FourCCMP3:
	default:
		return fmt.Errorf("unsupported fourCC: %v", m.FourCC)
	}

	m.AudioChannelOrder = AudioExChannelOrder(raw.Body[5])
	m.ChannelCount = raw.Body[6]

	switch m.AudioChannelOrder {
	case AudioExChannelOrderCustom:
		if len(raw.Body) != 8 {
			return fmt.Errorf("invalid AudioExMultichannelConfig size")
		}
		m.AudioChannelMapping = raw.Body[7]

	case AudioExChannelOrderNative:
		if len(raw.Body) != 11 {
			return fmt.Errorf("invalid AudioExMultichannelConfig size")
		}
		m.AudioChannelFlags = uint32(raw.Body[7])<<24 | uint32(raw.Body[8])<<16 |
			uint32(raw.Body[9])<<8 | uint32(raw.Body[10])

	case AudioExChannelOrderUnspecified:
		if len(raw.Body) != 7 {
			return fmt.Errorf("invalid AudioExMultichannelConfig size")
		}

	default:
		return fmt.Errorf("invalid AudioChannelOrder: %v", m.AudioChannelOrder)
	}

	return nil
}

func (m AudioExMultichannelConfig) marshal() (*rawmessage.Message, error) {
	var addBody []byte

	switch m.AudioChannelOrder {
	case AudioExChannelOrderCustom:
		addBody = []byte{m.AudioChannelMapping}

	case AudioExChannelOrderNative:
		addBody = []byte{
			byte(m.AudioChannelFlags >> 24),
			byte(m.AudioChannelFlags >> 16),
			byte(m.AudioChannelFlags >> 8),
			byte(m.AudioChannelFlags),
		}
	}

	body := make([]byte, 7+len(addBody))

	body[0] = (9 << 4) | byte(AudioExTypeMultichannelConfig)
	body[1] = uint8(m.FourCC >> 24)
	body[2] = uint8(m.FourCC >> 16)
	body[3] = uint8(m.FourCC >> 8)
	body[4] = uint8(m.FourCC)
	body[5] = uint8(m.AudioChannelOrder)
	body[6] = m.ChannelCount
	copy(body[7:], addBody)

	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Type:            uint8(TypeAudio),
		MessageStreamID: m.MessageStreamID,
		Body:            body,
	}, nil
}
