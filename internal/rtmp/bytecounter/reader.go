package bytecounter

import (
	"bufio"
	"io"
)

type readerInner struct {
	r     io.Reader
	count uint32
}

func (r *readerInner) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.count += uint32(n)
	return n, err
}

// Reader allows to count read bytes.
type Reader struct {
	ri *readerInner
	*bufio.Reader
}

// NewReader allocates a Reader.
func NewReader(r io.Reader) *Reader {
	ri := &readerInner{r: r}
	return &Reader{
		ri:     ri,
		Reader: bufio.NewReader(ri),
	}
}

// Count returns read bytes.
func (r Reader) Count() uint32 {
	return r.ri.count
}
