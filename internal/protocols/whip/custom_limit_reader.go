package whip

import (
	"fmt"
	"io"
)

var errSizeExceeded = fmt.Errorf("size exceeds maximum allowed")

// like io.LimitReader, but returns a dedicated error if the limit is exceeded.
type customLimitReader struct {
	r io.Reader
	n int64
}

func (l *customLimitReader) Read(p []byte) (n int, err error) {
	if l.n <= 0 {
		return 0, errSizeExceeded
	}
	if int64(len(p)) > l.n {
		p = p[0:l.n]
	}
	n, err = l.r.Read(p)
	l.n -= int64(n)
	return
}
