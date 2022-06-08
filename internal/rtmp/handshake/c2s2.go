package handshake

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
)

// C2S2 is a C2 or S2 packet.
type C2S2 struct {
	Time   uint32
	Time2  uint32
	Random []byte
	Digest []byte
}

// Read reads a C2S2.
func (c *C2S2) Read(r io.Reader) error {
	buf := make([]byte, 1536)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return err
	}

	// validate signature
	gap := len(buf) - 32
	digest := hsMakeDigest(c.Digest, buf, gap)
	if !bytes.Equal(buf[gap:gap+32], digest) {
		return fmt.Errorf("unable to validate C2/S2 signature")
	}

	c.Time = binary.BigEndian.Uint32(buf)
	c.Time2 = binary.BigEndian.Uint32(buf[4:])
	c.Random = buf[8:]

	return nil
}

// Write writes a C2S2.
func (c C2S2) Write(w io.Writer) error {
	buf := make([]byte, 1536)
	binary.BigEndian.PutUint32(buf, c.Time)
	binary.BigEndian.PutUint32(buf[4:], c.Time2)

	if c.Random == nil {
		rand.Read(buf[8:])
	} else {
		copy(buf[8:], c.Random)
	}

	// signature
	if c.Digest != nil {
		gap := len(buf) - 32
		digest := hsMakeDigest(c.Digest, buf, gap)
		copy(buf[gap:], digest)
	}

	_, err := w.Write(buf)
	return err
}
