package message

import (
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/amf0"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/rawmessage"
)

// DataAMF0 is a AMF0 data message.
type DataAMF0 struct {
	ChunkStreamID   byte
	MessageStreamID uint32
	Payload         []interface{}
}

func (m *DataAMF0) unmarshal(raw *rawmessage.Message) error {
	m.ChunkStreamID = raw.ChunkStreamID
	m.MessageStreamID = raw.MessageStreamID

	payload, err := amf0.Unmarshal(raw.Body)
	if err != nil {
		return err
	}
	m.Payload = payload

	return nil
}

func (m DataAMF0) marshal() (*rawmessage.Message, error) {
	body, err := amf0.Marshal(m.Payload)
	if err != nil {
		return nil, err
	}

	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Type:            uint8(TypeDataAMF0),
		MessageStreamID: m.MessageStreamID,
		Body:            body,
	}, nil
}
