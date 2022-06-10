package message

import (
	"github.com/aler9/rtsp-simple-server/internal/rtmp/bytecounter"
)

// ReadWriter is a message reader/writer.
type ReadWriter struct {
	r *Reader
	w *Writer
}

// NewReadWriter allocates a ReadWriter.
func NewReadWriter(bc *bytecounter.ReadWriter) *ReadWriter {
	w := NewWriter(bc.Writer)

	r := NewReader(bc.Reader, func(count uint32) error {
		return w.Write(&MsgAcknowledge{
			Value: count,
		})
	})

	return &ReadWriter{
		r: r,
		w: w,
	}
}

// Read reads a message.
func (rw *ReadWriter) Read() (Message, error) {
	msg, err := rw.r.Read()
	if err != nil {
		return nil, err
	}

	switch tmsg := msg.(type) {
	case *MsgAcknowledge:
		rw.w.SetAcknowledgeValue(tmsg.Value)

	case *MsgUserControlPingRequest:
		rw.w.Write(&MsgUserControlPingRequest{
			ServerTime: tmsg.ServerTime,
		})
	}

	return msg, nil
}

// Write writes a message.
func (rw *ReadWriter) Write(msg Message) error {
	return rw.w.Write(msg)
}
