package message

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/amf0"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/rawmessage"
)

// CommandAMF0 is a AMF0 command message.
type CommandAMF0 struct {
	ChunkStreamID   byte
	MessageStreamID uint32
	Name            string
	CommandID       int
	Arguments       amf0.Data
}

func (m *CommandAMF0) unmarshal(raw *rawmessage.Message) error {
	m.ChunkStreamID = raw.ChunkStreamID
	m.MessageStreamID = raw.MessageStreamID

	payload, err := amf0.Unmarshal(raw.Body)
	if err != nil {
		return err
	}

	if len(payload) < 3 {
		return fmt.Errorf("invalid command payload")
	}

	var ok bool
	m.Name, ok = payload[0].(string)
	if !ok {
		return fmt.Errorf("invalid command payload")
	}

	tmp, ok := payload[1].(float64)
	if !ok {
		return fmt.Errorf("invalid command payload")
	}
	m.CommandID = int(tmp)

	m.Arguments = payload[2:]

	return nil
}

func (m CommandAMF0) marshal() (*rawmessage.Message, error) {
	data := append(amf0.Data{
		m.Name,
		float64(m.CommandID),
	}, m.Arguments...)

	body, err := data.Marshal()
	if err != nil {
		return nil, err
	}

	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Type:            uint8(TypeCommandAMF0),
		MessageStreamID: m.MessageStreamID,
		Body:            body,
	}, nil
}
