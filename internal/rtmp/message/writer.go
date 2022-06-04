package message

import (
	"io"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/rawmessage"
)

// Writer is a message writer.
type Writer struct {
	w *rawmessage.Writer
}

// NewWriter allocates a Writer.
func NewWriter(w io.Writer) *Writer {
	return &Writer{
		w: rawmessage.NewWriter(w),
	}
}

// SetChunkSize sets the maximum chunk size.
func (mw *Writer) SetChunkSize(v int) {
	mw.w.SetChunkSize(v)
}

// Write writes a message.
func (mw *Writer) Write(msg Message) error {
	raw, err := msg.Marshal()
	if err != nil {
		return err
	}

	return mw.w.Write(raw)
}
