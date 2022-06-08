package message

import (
	"github.com/aler9/rtsp-simple-server/internal/rtmp/bytecounter"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/rawmessage"
)

// Writer is a message writer.
type Writer struct {
	w *rawmessage.Writer
}

// NewWriter allocates a Writer.
func NewWriter(w *bytecounter.Writer) *Writer {
	return &Writer{
		w: rawmessage.NewWriter(w),
	}
}

// SetAcknowledgeValue sets the value of the last received acknowledge.
func (w *Writer) SetAcknowledgeValue(v uint32) {
	w.w.SetAcknowledgeValue(v)
}

// Write writes a message.
func (w *Writer) Write(msg Message) error {
	raw, err := msg.Marshal()
	if err != nil {
		return err
	}

	err = w.w.Write(raw)
	if err != nil {
		return err
	}

	switch tmsg := msg.(type) {
	case *MsgSetChunkSize:
		w.w.SetChunkSize(tmsg.Value)

	case *MsgSetWindowAckSize:
		w.w.SetWindowAckSize(tmsg.Value)
	}

	return nil
}
