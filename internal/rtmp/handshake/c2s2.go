package handshake

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
)

const (
	c2s2Size      = c1s1Size
	c2s2DigestPos = c2s2Size - 8 - digestLength
)

var (
	randomCrud = []byte{
		0xf0, 0xee, 0xc2, 0x4a, 0x80, 0x68, 0xbe, 0xe8,
		0x2e, 0x00, 0xd0, 0xd1, 0x02, 0x9e, 0x7e, 0x57,
		0x6e, 0xec, 0x5d, 0x2d, 0x29, 0x80, 0x6f, 0xab,
		0x93, 0xb8, 0xe6, 0x36, 0xcf, 0xeb, 0x31, 0xae,
	}
	clientKeyC2 = append([]byte(nil), append(clientKeyC1, randomCrud...)...)
	serverKeyS2 = append([]byte(nil), append(serverKeyS1, randomCrud...)...)
)

// C2S2 is a C2 or S2 packet.
type C2S2 struct {
	Time  uint32
	Time2 uint32
	Data  []byte
}

// Read reads a C2S2.
func (c *C2S2) Read(r io.Reader) error {
	buf := make([]byte, c2s2Size)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return err
	}

	c.Time = uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
	c.Time2 = uint32(buf[4])<<24 | uint32(buf[5])<<16 | uint32(buf[6])<<8 | uint32(buf[7])
	c.Data = buf[8:]

	return nil
}

func (c C2S2) computeDigest(isS2 bool, prevDigest []byte) []byte {
	// hash entire message except digest
	msg := make([]byte, c2s2Size-digestLength)
	msg[0] = byte(c.Time >> 24)
	msg[1] = byte(c.Time >> 16)
	msg[2] = byte(c.Time >> 8)
	msg[3] = byte(c.Time)
	msg[4] = byte(c.Time2 >> 24)
	msg[5] = byte(c.Time2 >> 16)
	msg[6] = byte(c.Time2 >> 8)
	msg[7] = byte(c.Time2)
	copy(msg[8:], c.Data[:c2s2DigestPos])

	var key []byte
	if isS2 {
		key = hmacSha256(serverKeyS2, prevDigest)
	} else {
		key = hmacSha256(clientKeyC2, prevDigest)
	}

	return hmacSha256(key, msg)
}

func (c C2S2) validate(isS2 bool, prevDigest []byte) error {
	d1 := c.Data[c2s2DigestPos : c2s2DigestPos+digestLength]
	d2 := c.computeDigest(isS2, prevDigest)

	if !bytes.Equal(d1, d2) {
		return fmt.Errorf("unable to validate C2/S2 digest")
	}

	return nil
}

func (c *C2S2) fillPlain() error {
	c.Data = make([]byte, c2s2Size-8)
	_, err := rand.Read(c.Data)
	return err
}

func (c *C2S2) fill(isS2 bool, prevDigest []byte) error {
	err := c.fillPlain()
	if err != nil {
		return err
	}

	digest := c.computeDigest(isS2, prevDigest)
	copy(c.Data[c2s2DigestPos:], digest)
	return nil
}

// Write writes a C2S2.
func (c C2S2) Write(w io.Writer) error {
	buf := make([]byte, c2s2Size)

	buf[0] = byte(c.Time >> 24)
	buf[1] = byte(c.Time >> 16)
	buf[2] = byte(c.Time >> 8)
	buf[3] = byte(c.Time)
	buf[4] = byte(c.Time2 >> 24)
	buf[5] = byte(c.Time2 >> 16)
	buf[6] = byte(c.Time2 >> 8)
	buf[7] = byte(c.Time2)
	copy(buf[8:], c.Data)

	_, err := w.Write(buf)
	return err
}
