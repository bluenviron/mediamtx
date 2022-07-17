package handshake

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
)

var (
	hsClientFullKey = []byte{
		'G', 'e', 'n', 'u', 'i', 'n', 'e', ' ', 'A', 'd', 'o', 'b', 'e', ' ',
		'F', 'l', 'a', 's', 'h', ' ', 'P', 'l', 'a', 'y', 'e', 'r', ' ',
		'0', '0', '1',
		0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8, 0x2E, 0x00, 0xD0, 0xD1,
		0x02, 0x9E, 0x7E, 0x57, 0x6E, 0xEC, 0x5D, 0x2D, 0x29, 0x80, 0x6F, 0xAB,
		0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB, 0x31, 0xAE,
	}
	hsServerFullKey = []byte{
		'G', 'e', 'n', 'u', 'i', 'n', 'e', ' ', 'A', 'd', 'o', 'b', 'e', ' ',
		'F', 'l', 'a', 's', 'h', ' ', 'M', 'e', 'd', 'i', 'a', ' ',
		'S', 'e', 'r', 'v', 'e', 'r', ' ',
		'0', '0', '1',
		0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8, 0x2E, 0x00, 0xD0, 0xD1,
		0x02, 0x9E, 0x7E, 0x57, 0x6E, 0xEC, 0x5D, 0x2D, 0x29, 0x80, 0x6F, 0xAB,
		0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB, 0x31, 0xAE,
	}
	hsClientPartialKey = hsClientFullKey[:30]
	hsServerPartialKey = hsServerFullKey[:36]
)

func hsCalcDigestPos(p []byte, base int) int {
	pos := 0
	for i := 0; i < 4; i++ {
		pos += int(p[base+i])
	}
	return (pos % 728) + base + 4
}

func hsMakeDigest(key []byte, src []byte, gap int) []byte {
	h := hmac.New(sha256.New, key)
	if gap <= 0 {
		h.Write(src)
	} else {
		h.Write(src[:gap])
		h.Write(src[gap+32:])
	}
	return h.Sum(nil)
}

func hsFindDigest(p []byte, key []byte, base int) int {
	gap := hsCalcDigestPos(p, base)
	digest := hsMakeDigest(key, p, gap)
	if !bytes.Equal(p[gap:gap+32], digest) {
		return -1
	}
	return gap
}

func hsParse1(p []byte, peerkey []byte, key []byte) (bool, []byte) {
	var pos int
	if pos = hsFindDigest(p, peerkey, 772); pos == -1 {
		if pos = hsFindDigest(p, peerkey, 8); pos == -1 {
			return false, nil
		}
	}
	return true, hsMakeDigest(key, p[pos:pos+32], -1)
}

// C1S1 is a C1 or S1 packet.
type C1S1 struct {
	Time   uint32
	Random []byte
	Digest []byte
}

// Read reads a C1S1.
func (c *C1S1) Read(r io.Reader, isC1 bool, validateSignature bool) error {
	buf := make([]byte, 1536)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return err
	}

	var peerKey []byte
	var key []byte
	if isC1 {
		peerKey = hsClientPartialKey
		key = hsServerFullKey
	} else {
		peerKey = hsServerPartialKey
		key = hsClientFullKey
	}
	ok, digest := hsParse1(buf, peerKey, key)
	if !ok {
		if validateSignature {
			return fmt.Errorf("unable to validate C1/S1 signature")
		}
	} else {
		c.Digest = digest
	}

	c.Time = uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
	c.Random = buf[8:]

	return nil
}

// Write writes a C1S1.
func (c *C1S1) Write(w io.Writer, isC1 bool) error {
	buf := make([]byte, 1536)

	buf[0] = byte(c.Time >> 24)
	buf[1] = byte(c.Time >> 16)
	buf[2] = byte(c.Time >> 8)
	buf[3] = byte(c.Time)
	copy(buf[4:], []byte{0, 0, 0, 0})

	if c.Random == nil {
		rand.Read(buf[8:])
	} else {
		copy(buf[8:], c.Random)
	}

	// signature
	gap := hsCalcDigestPos(buf, 8)
	var peerKey []byte
	var key []byte
	if isC1 {
		peerKey = hsServerFullKey
		key = hsClientPartialKey
	} else {
		peerKey = hsClientFullKey
		key = hsServerPartialKey
	}
	digest := hsMakeDigest(key, buf, gap)
	copy(buf[gap:], digest)
	pos := hsFindDigest(buf, key, 8)
	c.Digest = hsMakeDigest(peerKey, buf[pos:pos+32], -1)

	_, err := w.Write(buf)
	return err
}
