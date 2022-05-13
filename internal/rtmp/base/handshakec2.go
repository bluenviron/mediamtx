package base

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
)

func hsFindDigest(p []byte, key []byte, base int) int {
	gap := hsCalcDigestPos(p, base)
	digest := hsMakeDigest(key, p, gap)
	if !bytes.Equal(p[gap:gap+32], digest) {
		return -1
	}
	return gap
}

func hsParse1(p []byte, peerkey []byte, key []byte) (ok bool, digest []byte) {
	var pos int
	if pos = hsFindDigest(p, peerkey, 772); pos == -1 {
		if pos = hsFindDigest(p, peerkey, 8); pos == -1 {
			return
		}
	}
	ok = true
	digest = hsMakeDigest(key, p[pos:pos+32], -1)
	return
}

// HandshakeC2 is the C2 part of an handshake.
type HandshakeC2 struct{}

// Read reads a HandshakeC2.
func (HandshakeC2) Write(w io.Writer, s1s2 []byte) error {
	ok, key := hsParse1(s1s2[:1536], hsServerPartialKey, hsClientFullKey)
	if !ok {
		return fmt.Errorf("unable to parse S1+S2")
	}

	buf := make([]byte, 1536)
	rand.Read(buf)
	gap := len(buf) - 32
	digest := hsMakeDigest(key, buf, gap)
	copy(buf[gap:], digest)
	_, err := w.Write(buf)
	return err
}
