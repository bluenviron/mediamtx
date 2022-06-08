package bytecounter

import (
	"io"
)

// Writer allows to count written bytes.
type Writer struct {
	w     io.Writer
	count uint32
}

// NewWriter allocates a Writer.
func NewWriter(w io.Writer) *Writer {
	return &Writer{
		w: w,
	}
}

// Write implements io.Writer.
func (w *Writer) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.count += uint32(n)
	return n, err
}

// Count returns written bytes.
func (w Writer) Count() uint32 {
	return w.count
}
