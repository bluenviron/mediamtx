package handshake

import (
	"bytes"
	"crypto/rand"
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
func (c *C2S2) Read(r io.Reader, validateSignature bool) error {
	buf := make([]byte, 1536)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return err
	}

	if validateSignature {
		gap := len(buf) - 32
		digest := hsMakeDigest(c.Digest, buf, gap)
		if !bytes.Equal(buf[gap:gap+32], digest) {
			return fmt.Errorf("unable to validate C2/S2 signature")
		}
	}

	c.Time = uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
	c.Time2 = uint32(buf[4])<<24 | uint32(buf[5])<<16 | uint32(buf[6])<<8 | uint32(buf[7])
	c.Random = buf[8:]

	return nil
}

// Write writes a C2S2.
func (c C2S2) Write(w io.Writer) error {
	buf := make([]byte, 1536)

	buf[0] = byte(c.Time >> 24)
	buf[1] = byte(c.Time >> 16)
	buf[2] = byte(c.Time >> 8)
	buf[3] = byte(c.Time)
	buf[4] = byte(c.Time2 >> 24)
	buf[5] = byte(c.Time2 >> 16)
	buf[6] = byte(c.Time2 >> 8)
	buf[7] = byte(c.Time2)

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
