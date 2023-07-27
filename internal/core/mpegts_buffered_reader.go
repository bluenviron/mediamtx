package core

import (
	"fmt"
	"io"
)

// mpegtsBufferedReader is a buffered reader optimized for MPEG-TS.
type mpegtsBufferedReader struct {
	r         io.Reader
	midbuf    []byte
	midbufpos int
}

func newMPEGTSBufferedReader(r io.Reader) *mpegtsBufferedReader {
	return &mpegtsBufferedReader{
		r:      r,
		midbuf: make([]byte, 0, 1500),
	}
}

// Read implements io.Reader.
func (r *mpegtsBufferedReader) Read(p []byte) (int, error) {
	if r.midbufpos < len(r.midbuf) {
		n := copy(p, r.midbuf[r.midbufpos:])
		r.midbufpos += n
		return n, nil
	}

	mn, err := r.r.Read(r.midbuf[:cap(r.midbuf)])
	if err != nil {
		return 0, err
	}

	if (mn % 188) != 0 {
		return 0, fmt.Errorf("received packet with size %d not multiple of 188", mn)
	}

	r.midbuf = r.midbuf[:mn]
	n := copy(p, r.midbuf)
	r.midbufpos = n
	return n, nil
}
