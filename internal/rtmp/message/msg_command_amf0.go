package message

import (
	"github.com/notedit/rtmp/format/flv/flvio"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/rawmessage"
)

// MsgCommandAMF0 is a AMF0 command message.
type MsgCommandAMF0 struct {
	ChunkStreamID   byte
	MessageStreamID uint32
	Payload         []interface{}
}

// Unmarshal implements Message.
func (m *MsgCommandAMF0) Unmarshal(raw *rawmessage.Message) error {
	m.ChunkStreamID = raw.ChunkStreamID
	m.MessageStreamID = raw.MessageStreamID

	payload, err := flvio.ParseAMFVals(raw.Body, false)
	if err != nil {
		return err
	}
	m.Payload = payload

	return nil
}

// Marshal implements Message.
func (m MsgCommandAMF0) Marshal() (*rawmessage.Message, error) {
	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Type:            chunk.MessageTypeCommandAMF0,
		MessageStreamID: m.MessageStreamID,
		Body:            flvio.FillAMF0ValsMalloc(m.Payload),
	}, nil
}
