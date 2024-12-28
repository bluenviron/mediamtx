package message //nolint:dupl

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/rawmessage"
)

// AudioExMultitrackType is a multitrack type.
type AudioExMultitrackType uint8

// multitrack types.
const (
	AudioExMultitrackTypeOneTrack             AudioExMultitrackType = 0
	AudioExMultitrackTypeManyTracks           AudioExMultitrackType = 1
	AudioExMultitrackTypeManyTracksManyCodecs AudioExMultitrackType = 2
)

// AudioExMultitrack is a multitrack extended message.
type AudioExMultitrack struct {
	MultitrackType AudioExMultitrackType
	TrackID        uint8
	Wrapped        Message
}

func (m *AudioExMultitrack) unmarshal(raw *rawmessage.Message) error { //nolint:dupl
	if len(raw.Body) < 7 {
		return fmt.Errorf("not enough bytes")
	}

	m.MultitrackType = AudioExMultitrackType(raw.Body[1] >> 4)
	switch m.MultitrackType {
	case AudioExMultitrackTypeOneTrack:
	default:
		return fmt.Errorf("unsupported multitrack type: %v", m.MultitrackType)
	}

	packetType := AudioExType(raw.Body[1] & 0b1111)
	switch packetType {
	case AudioExTypeSequenceStart:
		m.Wrapped = &AudioExSequenceStart{}

	case AudioExTypeSequenceEnd:
		m.Wrapped = &AudioExSequenceEnd{}

	case AudioExTypeMultichannelConfig:
		m.Wrapped = &AudioExMultichannelConfig{}

	case AudioExTypeCodedFrames:
		m.Wrapped = &AudioExCodedFrames{}

	default:
		return fmt.Errorf("unsupported audio multitrack packet type: %v", packetType)
	}

	m.TrackID = raw.Body[6]

	wrappedBody := make([]byte, 5+len(raw.Body[7:]))
	copy(wrappedBody[1:], raw.Body[2:]) // fourCC
	copy(wrappedBody[5:], raw.Body[7:]) // body
	err := m.Wrapped.unmarshal(&rawmessage.Message{
		ChunkStreamID:   raw.ChunkStreamID,
		MessageStreamID: raw.MessageStreamID,
		Timestamp:       raw.Timestamp,
		Body:            wrappedBody,
	})
	if err != nil {
		return err
	}

	return nil
}

func (m AudioExMultitrack) marshal() (*rawmessage.Message, error) {
	wrappedEnc, err := m.Wrapped.marshal()
	if err != nil {
		return nil, err
	}

	body := make([]byte, 7+len(wrappedEnc.Body)-5)

	body[0] = (9 << 4) | byte(AudioExTypeMultitrack)
	body[1] = wrappedEnc.Body[0] & 0b1111
	copy(body[2:], wrappedEnc.Body[1:])
	body[6] = m.TrackID
	copy(body[7:], wrappedEnc.Body[5:])

	return &rawmessage.Message{
		ChunkStreamID:   wrappedEnc.ChunkStreamID,
		MessageStreamID: wrappedEnc.MessageStreamID,
		Timestamp:       wrappedEnc.Timestamp,
		Type:            uint8(TypeAudio),
		Body:            body,
	}, nil
}
