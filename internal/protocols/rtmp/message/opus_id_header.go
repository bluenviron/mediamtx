package message

import (
	"bytes"
	"fmt"
)

var magicSignature = []byte{'O', 'p', 'u', 's', 'H', 'e', 'a', 'd'}

// OpusIDHeader is an Opus identification header.
// Specification: RFC7845, section 5.1
type OpusIDHeader struct {
	Version              uint8
	ChannelCount         uint8
	PreSkip              uint16
	InputSampleRate      uint32
	OutputGain           uint16
	ChannelMappingFamily uint8
	ChannelMappingTable  []uint8
}

func (h *OpusIDHeader) unmarshal(buf []byte) error {
	if len(buf) < 19 {
		return fmt.Errorf("not enough bytes")
	}

	if !bytes.Equal(buf[:8], magicSignature) {
		return fmt.Errorf("magic signature not corresponds")
	}

	h.Version = buf[8]
	if h.Version != 1 {
		return fmt.Errorf("invalid version: %v", h.Version)
	}

	h.ChannelCount = buf[9]
	h.PreSkip = uint16(buf[10])<<8 | uint16(buf[11])
	h.InputSampleRate = uint32(buf[12])<<24 | uint32(buf[13])<<16 | uint32(buf[14])<<8 | uint32(buf[15])
	h.OutputGain = uint16(buf[16])<<8 | uint16(buf[17])
	h.ChannelMappingFamily = buf[18]
	h.ChannelMappingTable = buf[19:]

	return nil
}

func (h OpusIDHeader) marshalSize() int {
	return 19 + len(h.ChannelMappingTable)
}

func (h OpusIDHeader) marshalTo(buf []byte) (int, error) {
	copy(buf[0:], magicSignature)
	buf[8] = 1
	buf[9] = h.ChannelCount
	buf[10] = byte(h.PreSkip >> 8)
	buf[11] = byte(h.PreSkip)
	buf[12] = byte(h.InputSampleRate >> 24)
	buf[13] = byte(h.InputSampleRate >> 16)
	buf[14] = byte(h.InputSampleRate >> 8)
	buf[15] = byte(h.InputSampleRate)
	buf[16] = byte(h.OutputGain >> 8)
	buf[17] = byte(h.OutputGain)
	buf[18] = h.ChannelMappingFamily
	n := copy(buf[19:], h.ChannelMappingTable)
	return 19 + n, nil
}

func (h OpusIDHeader) marshal() ([]byte, error) {
	buf := make([]byte, h.marshalSize())

	_, err := h.marshalTo(buf)
	if err != nil {
		return nil, err
	}

	return buf, nil
}
