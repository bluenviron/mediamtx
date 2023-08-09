package handshake

import (
	"fmt"
	"io"
)

// C0S0 is a C0 or S0 packet.
type C0S0 struct {
	Version byte
}

// Read reads a C0S0.
func (c *C0S0) Read(r io.Reader) error {
	buf := make([]byte, 1)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return err
	}

	c.Version = buf[0]

	if c.Version != 3 && c.Version != 6 {
		return fmt.Errorf("invalid rtmp version (%d)", c.Version)
	}

	return nil
}

// Write writes a C0S0.
func (c C0S0) Write(w io.Writer) error {
	_, err := w.Write([]byte{c.Version})
	return err
}
