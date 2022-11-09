package bytecounter

import (
	"io"
)

// Reader allows to count read bytes.
type Reader struct {
	r     io.Reader
	count uint32
}

// NewReader allocates a Reader.
func NewReader(r io.Reader) *Reader {
	return &Reader{
		r: r,
	}
}

// Read implements io.Reader.
func (r *Reader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.count += uint32(n)
	return n, err
}

// Count returns read bytes.
func (r Reader) Count() uint32 {
	return r.count
}

// SetCount sets read bytes.
func (r *Reader) SetCount(v uint32) {
	r.count = v
}
