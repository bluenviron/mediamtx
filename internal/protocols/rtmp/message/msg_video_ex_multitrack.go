package message //nolint:dupl

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/rawmessage"
)

// VideoExMultitrackType is a multitrack type.
type VideoExMultitrackType uint8

// multitrack types.
const (
	VideoExMultitrackTypeOneTrack             VideoExMultitrackType = 0
	VideoExMultitrackTypeManyTracks           VideoExMultitrackType = 1
	VideoExMultitrackTypeManyTracksManyCodecs VideoExMultitrackType = 2
)

// VideoExMultitrack is a multitrack extended message.
type VideoExMultitrack struct {
	MultitrackType VideoExMultitrackType
	TrackID        uint8
	Wrapped        Message
}

func (m *VideoExMultitrack) unmarshal(raw *rawmessage.Message) error { //nolint:dupl
	if len(raw.Body) < 7 {
		return fmt.Errorf("not enough bytes")
	}

	m.MultitrackType = VideoExMultitrackType(raw.Body[1] >> 4)
	switch m.MultitrackType {
	case VideoExMultitrackTypeOneTrack:
	default:
		return fmt.Errorf("unsupported multitrack type: %v", m.MultitrackType)
	}

	packetType := VideoExType(raw.Body[1] & 0b1111)
	switch packetType {
	case VideoExTypeSequenceStart:
		m.Wrapped = &VideoExSequenceStart{}

	case VideoExTypeSequenceEnd:
		m.Wrapped = &VideoExSequenceEnd{}

	case VideoExTypeCodedFrames:
		m.Wrapped = &VideoExCodedFrames{}

	case VideoExTypeFramesX:
		m.Wrapped = &VideoExFramesX{}

	default:
		return fmt.Errorf("unsupported video multitrack packet type: %v", packetType)
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

func (m VideoExMultitrack) marshal() (*rawmessage.Message, error) {
	wrappedEnc, err := m.Wrapped.marshal()
	if err != nil {
		return nil, err
	}

	body := make([]byte, 7+len(wrappedEnc.Body)-5)

	body[0] = 0b10000000 | byte(VideoExTypeMultitrack)
	body[1] = wrappedEnc.Body[0] & 0b1111
	copy(body[2:], wrappedEnc.Body[1:])
	body[6] = m.TrackID
	copy(body[7:], wrappedEnc.Body[5:])

	return &rawmessage.Message{
		ChunkStreamID:   wrappedEnc.ChunkStreamID,
		MessageStreamID: wrappedEnc.MessageStreamID,
		Timestamp:       wrappedEnc.Timestamp,
		Type:            uint8(TypeVideo),
		Body:            body,
	}, nil
}
