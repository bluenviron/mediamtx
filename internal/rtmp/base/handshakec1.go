package base

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"io"
)

func hsCalcDigestPos(p []byte, base int) (pos int) {
	for i := 0; i < 4; i++ {
		pos += int(p[base+i])
	}
	pos = (pos % 728) + base + 4
	return
}

func hsMakeDigest(key []byte, src []byte, gap int) (dst []byte) {
	h := hmac.New(sha256.New, key)
	if gap <= 0 {
		h.Write(src)
	} else {
		h.Write(src[:gap])
		h.Write(src[gap+32:])
	}
	return h.Sum(nil)
}

// HandshakeC1 is the C1 part of an handshake.
type HandshakeC1 struct{}

// Read reads a HandshakeC1.
func (HandshakeC1) Write(w io.Writer) error {
	buf := make([]byte, 1536)
	copy(buf[0:4], []byte{0x00, 0x00, 0x00, 0x00})
	copy(buf[4:8], []byte{0x09, 0x00, 0x7c, 0x02})

	rand.Read(buf[8:])
	gap := hsCalcDigestPos(buf[0:], 8)
	digest := hsMakeDigest(hsClientPartialKey, buf[0:], gap)
	copy(buf[gap+0:], digest)

	_, err := w.Write(buf[0:])
	return err
}
