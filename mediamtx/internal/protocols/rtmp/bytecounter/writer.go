package bytecounter

import (
	"io"
	"sync/atomic"
)

// Writer allows to count written bytes.
type Writer struct {
	w     io.Writer
	count uint64
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
	atomic.AddUint64(&w.count, uint64(n))
	return n, err
}

// Count returns sent bytes.
func (w *Writer) Count() uint64 {
	return atomic.LoadUint64(&w.count)
}

// SetCount sets sent bytes.
func (w *Writer) SetCount(v uint64) {
	atomic.StoreUint64(&w.count, v)
}
