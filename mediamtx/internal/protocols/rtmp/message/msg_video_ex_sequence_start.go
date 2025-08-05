package message

import (
	"bytes"
	"fmt"

	"github.com/abema/go-mp4"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/rawmessage"
)

// VideoExSequenceStart is a sequence start extended message.
type VideoExSequenceStart struct {
	ChunkStreamID   byte
	MessageStreamID uint32
	FourCC          FourCC
	AV1Header       *mp4.Av1C
	VP9Header       *mp4.VpcC
	HEVCHeader      *mp4.HvcC
	AVCHeader       *mp4.AVCDecoderConfiguration
}

func (m *VideoExSequenceStart) unmarshal(raw *rawmessage.Message) error {
	if len(raw.Body) < 5 {
		return fmt.Errorf("not enough bytes")
	}

	m.ChunkStreamID = raw.ChunkStreamID
	m.MessageStreamID = raw.MessageStreamID

	m.FourCC = FourCC(raw.Body[1])<<24 | FourCC(raw.Body[2])<<16 | FourCC(raw.Body[3])<<8 | FourCC(raw.Body[4])
	switch m.FourCC {
	case FourCCAV1:
		m.AV1Header = &mp4.Av1C{}
		_, err := mp4.Unmarshal(bytes.NewReader(raw.Body[5:]), uint64(len(raw.Body[5:])), m.AV1Header, mp4.Context{})
		if err != nil {
			return fmt.Errorf("invalid AV1 configuration: %w", err)
		}

	case FourCCVP9:
		m.VP9Header = &mp4.VpcC{}
		_, err := mp4.Unmarshal(bytes.NewReader(raw.Body[5:]), uint64(len(raw.Body[5:])), m.VP9Header, mp4.Context{})
		if err != nil {
			return fmt.Errorf("invalid VP9 configuration: %w", err)
		}

	case FourCCHEVC:
		m.HEVCHeader = &mp4.HvcC{}
		_, err := mp4.Unmarshal(bytes.NewReader(raw.Body[5:]), uint64(len(raw.Body[5:])), m.HEVCHeader, mp4.Context{})
		if err != nil {
			return fmt.Errorf("invalid H265 configuration: %w", err)
		}

	case FourCCAVC:
		m.AVCHeader = &mp4.AVCDecoderConfiguration{}
		m.AVCHeader.SetType(mp4.BoxTypeAvcC())
		_, err := mp4.Unmarshal(bytes.NewReader(raw.Body[5:]), uint64(len(raw.Body[5:])), m.AVCHeader, mp4.Context{})
		if err != nil {
			return fmt.Errorf("invalid H264 configuration: %w", err)
		}

	default:
		return fmt.Errorf("unsupported fourCC: %v", m.FourCC)
	}

	return nil
}

func (m VideoExSequenceStart) marshal() (*rawmessage.Message, error) {
	var addBody []byte

	switch m.FourCC {
	case FourCCAV1:
		var buf bytes.Buffer
		_, err := mp4.Marshal(&buf, m.AV1Header, mp4.Context{})
		if err != nil {
			return nil, err
		}
		addBody = buf.Bytes()

	case FourCCVP9:
		var buf bytes.Buffer
		_, err := mp4.Marshal(&buf, m.VP9Header, mp4.Context{})
		if err != nil {
			return nil, err
		}
		addBody = buf.Bytes()

	case FourCCHEVC:
		var buf bytes.Buffer
		_, err := mp4.Marshal(&buf, m.HEVCHeader, mp4.Context{})
		if err != nil {
			return nil, err
		}
		addBody = buf.Bytes()

	case FourCCAVC:
		var buf bytes.Buffer
		_, err := mp4.Marshal(&buf, m.AVCHeader, mp4.Context{})
		if err != nil {
			return nil, err
		}
		addBody = buf.Bytes()
	}

	body := make([]byte, 5+len(addBody))

	body[0] = 0b10000000 | byte(VideoExTypeSequenceStart)
	body[1] = uint8(m.FourCC >> 24)
	body[2] = uint8(m.FourCC >> 16)
	body[3] = uint8(m.FourCC >> 8)
	body[4] = uint8(m.FourCC)
	copy(body[5:], addBody)

	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Type:            uint8(TypeVideo),
		MessageStreamID: m.MessageStreamID,
		Body:            body,
	}, nil
}
