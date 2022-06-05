package handshake

import (
	"crypto/rand"
	"encoding/binary"
	"io"
)

// C2S2 is a C2 or S2 packet.
type C2S2 struct {
	Time   uint32
	Time2  uint32
	Random []byte
}

// Read reads a C2S2.
func (c *C2S2) Read(r io.Reader) error {
	buf := make([]byte, 1536)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return err
	}

	c.Time = binary.BigEndian.Uint32(buf)
	c.Time2 = binary.BigEndian.Uint32(buf[4:])
	c.Random = buf[8:]

	return nil
}

// Write writes a C2S2.
func (c C2S2) Write(w io.Writer, key []byte) error {
	buf := make([]byte, 1536)
	binary.BigEndian.PutUint32(buf, c.Time)
	binary.BigEndian.PutUint32(buf[4:], c.Time2)

	if c.Random == nil {
		rand.Read(buf[8:])
	} else {
		copy(buf[8:], c.Random)
	}

	// signature
	gap := len(buf) - 32
	digest := hsMakeDigest(key, buf, gap)
	copy(buf[gap:], digest)

	_, err := w.Write(buf)
	return err
}
