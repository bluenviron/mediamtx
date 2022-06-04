package handshake

import (
	"fmt"
	"io"
)

// S0 is the S0 part of an handshake.
type S0 struct{}

// Read reads a S0.
func (S0) Read(r io.Reader) error {
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
