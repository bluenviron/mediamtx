package message

import (
	"fmt"

	"github.com/notedit/rtmp/format/flv/flvio"

	"github.com/bluenviron/mediamtx/internal/rtmp/rawmessage"
)

// CommandAMF0 is a AMF0 command message.
type CommandAMF0 struct {
	ChunkStreamID   byte
	MessageStreamID uint32
	Name            string
	CommandID       int
	Arguments       []interface{}
}

// Unmarshal implements Message.
func (m *CommandAMF0) Unmarshal(raw *rawmessage.Message) error {
	m.ChunkStreamID = raw.ChunkStreamID
	m.MessageStreamID = raw.MessageStreamID

	payload, err := flvio.ParseAMFVals(raw.Body, false)
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

// Marshal implements Message.
func (m CommandAMF0) Marshal() (*rawmessage.Message, error) {
	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Type:            uint8(TypeCommandAMF0),
		MessageStreamID: m.MessageStreamID,
		Body: flvio.FillAMF0ValsMalloc(append([]interface{}{
			m.Name,
			float64(m.CommandID),
		}, m.Arguments...)),
	}, nil
}
