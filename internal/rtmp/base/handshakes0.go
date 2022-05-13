package base

import (
	"fmt"
	"io"
)

// HandshakeS0 is the S0 part of an handshake.
type HandshakeS0 struct{}

// Read reads a HandshakeS0.
func (HandshakeS0) Read(r io.Reader) error {
	buf := make([]byte, 1)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return err
	}

	if buf[0] != rtmpVersion {
		return fmt.Errorf("invalid rtmp version (%d)", buf[0])
	}

	return nil
}
