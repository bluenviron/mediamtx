package handshake

import (
	"fmt"
	"io"
)

const (
	rtmpVersion = 0x03
)

// C0S0 is a C0 or S0 packet.
type C0S0 struct{}

// Read reads a C0S0.
func (C0S0) Read(r io.Reader) error {
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

// Write writes a C0S0.
func (C0S0) Write(w io.Writer) error {
	_, err := w.Write([]byte{rtmpVersion})
	return err
}
