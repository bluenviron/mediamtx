package message

import (
	"fmt"

	"github.com/notedit/rtmp/format/flv/flvio"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/rawmessage"
)

// MsgCommandAMF0 is a AMF0 command message.
type MsgCommandAMF0 struct {
	ChunkStreamID   byte
	MessageStreamID uint32
	Name            string
	CommandID       int
	Arguments       []interface{}
}

// Unmarshal implements Message.
func (m *MsgCommandAMF0) Unmarshal(raw *rawmessage.Message) error {
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
func (m MsgCommandAMF0) Marshal() (*rawmessage.Message, error) {
	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Type:            chunk.MessageTypeCommandAMF0,
		MessageStreamID: m.MessageStreamID,
		Body: flvio.FillAMF0ValsMalloc(append([]interface{}{
			m.Name,
			float64(m.CommandID),
		}, m.Arguments...)),
	}, nil
}
