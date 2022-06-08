package bytecounter

import (
	"io"
)

// ReadWriter allows to count read and written bytes.
type ReadWriter struct {
	*Reader
	*Writer
}

// NewReadWriter allocates a ReadWriter.
func NewReadWriter(rw io.ReadWriter) *ReadWriter {
	return &ReadWriter{
		Reader: NewReader(rw),
		Writer: NewWriter(rw),
	}
}
