package message

import (
	"io"

	"github.com/bluenviron/mediamtx/internal/rtmp/bytecounter"
)

// ReadWriter is a message reader/writer.
type ReadWriter struct {
	r *Reader
	w *Writer
}

// NewReadWriter allocates a ReadWriter.
func NewReadWriter(
	rw io.ReadWriter,
	bcrw *bytecounter.ReadWriter,
	checkAcknowledge bool,
) *ReadWriter {
	w := NewWriter(rw, bcrw.Writer, checkAcknowledge)

	r := NewReader(rw, bcrw.Reader, func(count uint32) error {
		return w.Write(&Acknowledge{
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
	case *Acknowledge:
		rw.w.SetAcknowledgeValue(tmsg.Value)

	case *UserControlPingRequest:
		err := rw.w.Write(&UserControlPingResponse{
			ServerTime: tmsg.ServerTime,
		})
		if err != nil {
			return nil, err
		}
	}

	return msg, nil
}

// Write writes a message.
func (rw *ReadWriter) Write(msg Message) error {
	return rw.w.Write(msg)
}
