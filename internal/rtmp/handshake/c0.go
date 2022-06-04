package handshake

import (
	"io"
)

const (
	rtmpVersion = 0x03
)

// C0 is the C0 part of an handshake.
type C0 struct{}

// Read reads a C0.
func (C0) Write(w io.Writer) error {
	_, err := w.Write([]byte{rtmpVersion})
	return err
}
