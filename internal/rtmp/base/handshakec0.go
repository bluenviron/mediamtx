package base

import (
	"io"
)

// HandshakeC0 is the C0 part of an handshake.
type HandshakeC0 struct{}

// Read reads a HandshakeC0.
func (HandshakeC0) Write(w io.Writer) error {
	_, err := w.Write([]byte{rtmpVersion})
	return err
}
