package handshake

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
)

const (
	c1s1Size             = 1536
	digestPointerPos1    = 0
	digestPointerPos2    = 772 - 8
	digestChunkPos1      = digestPointerPos1 + 4
	digestChunkPos2      = digestPointerPos2 + 4
	digestChunkLength    = 728
	digestLength         = 32
	publicKeyPointerPos1 = 1532 - 8
	publicKeyPointerPos2 = 768 - 8
	publicKeyChunkPos1   = publicKeyPointerPos1 - 760
	publicKeyChunkPos2   = publicKeyPointerPos2 - 760
	publicKeyChunkLength = 632
)

var (
	clientKeyC1 = []byte("Genuine Adobe Flash Player 001")
	serverKeyS1 = []byte("Genuine Adobe Flash Media Server 001")
)

func hmacSha256(key []byte, buf []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(buf)
	return h.Sum(nil)
}

// C1S1 is a C1 or S1 packet.
type C1S1 struct {
	Time    uint32
	Version uint32
	Data    []byte
}

// Read reads a C1S1.
func (c *C1S1) Read(r io.Reader) error {
	buf := make([]byte, c1s1Size)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return err
	}

	c.Time = uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
	c.Version = uint32(buf[4])<<24 | uint32(buf[5])<<16 | uint32(buf[6])<<8 | uint32(buf[7])
	c.Data = buf[8:]

	return nil
}

func (c C1S1) readPointer(p int) int {
	return int(c.Data[p]) + int(c.Data[p+1]) + int(c.Data[p+2]) + int(c.Data[p+3])
}

func (c C1S1) publicKeyPos1() int {
	return publicKeyChunkPos1 + (c.readPointer(publicKeyPointerPos1) % publicKeyChunkLength)
}

func (c C1S1) publicKeyPos2() int {
	return publicKeyChunkPos2 + (c.readPointer(publicKeyPointerPos2) % publicKeyChunkLength)
}

func (c C1S1) digestPos1() int {
	return digestChunkPos1 + (c.readPointer(digestPointerPos1) % digestChunkLength)
}

func (c C1S1) digestPos2() int {
	return digestChunkPos2 + (c.readPointer(digestPointerPos2) % digestChunkLength)
}

func (c C1S1) computeDigest(digestPos int, isS1 bool) []byte {
	// hash entire message except digest
	msg := make([]byte, c1s1Size-digestLength)
	msg[0] = byte(c.Time >> 24)
	msg[1] = byte(c.Time >> 16)
	msg[2] = byte(c.Time >> 8)
	msg[3] = byte(c.Time)
	msg[4] = byte(c.Version >> 24)
	msg[5] = byte(c.Version >> 16)
	msg[6] = byte(c.Version >> 8)
	msg[7] = byte(c.Version)
	copy(msg[8:], c.Data[:digestPos])
	copy(msg[8+digestPos:], c.Data[digestPos+digestLength:])

	if isS1 {
		return hmacSha256(serverKeyS1, msg)
	}
	return hmacSha256(clientKeyC1, msg)
}

func (c C1S1) validateDigest(isS1 bool) ([]byte, []byte, error) {
	digestPos := c.digestPos1()
	d1 := c.Data[digestPos : digestPos+digestLength]
	d2 := c.computeDigest(digestPos, isS1)

	if bytes.Equal(d1, d2) {
		publicKeyPos := c.publicKeyPos1()
		publicKey := c.Data[publicKeyPos : publicKeyPos+dhKeyLength]
		return d1, publicKey, nil
	}

	digestPos = c.digestPos2()
	d1 = c.Data[digestPos : digestPos+digestLength]
	d2 = c.computeDigest(digestPos, isS1)

	if bytes.Equal(d1, d2) {
		publicKeyPos := c.publicKeyPos2()
		publicKey := c.Data[publicKeyPos : publicKeyPos+dhKeyLength]
		return d1, publicKey, nil
	}

	return nil, nil, fmt.Errorf("unable to validate C1/S1 digest")
}

func (c C1S1) validate(isS1 bool) ([]byte, []byte, error) {
	digest, publicKey, err := c.validateDigest(isS1)
	if err != nil {
		return nil, nil, err
	}

	err = dhValidatePublicKey(publicKey)
	if err != nil {
		return nil, nil, err
	}

	return digest, publicKey, nil
}

func (c *C1S1) fillPlain() error {
	c.Data = make([]byte, c1s1Size-8)
	_, err := rand.Read(c.Data)
	return err
}

func (c *C1S1) fill(isS1 bool, publicKey []byte) ([]byte, error) {
	err := c.fillPlain()
	if err != nil {
		return nil, err
	}

	var r [1]byte
	_, err = rand.Read(r[:])
	if err != nil {
		return nil, err
	}

	var digestPos int
	var publicKeyPos int

	if r[0] == 0 {
		digestPos = c.digestPos1()
		publicKeyPos = c.publicKeyPos1()
	} else {
		digestPos = c.digestPos2()
		publicKeyPos = c.publicKeyPos2()
	}

	copy(c.Data[publicKeyPos:], publicKey)
	digest := c.computeDigest(digestPos, isS1)
	copy(c.Data[digestPos:], digest)
	return digest, nil
}

// Write writes a C1S1.
func (c C1S1) Write(w io.Writer) error {
	buf := make([]byte, c1s1Size)

	buf[0] = byte(c.Time >> 24)
	buf[1] = byte(c.Time >> 16)
	buf[2] = byte(c.Time >> 8)
	buf[3] = byte(c.Time)
	buf[4] = byte(c.Version >> 24)
	buf[5] = byte(c.Version >> 16)
	buf[6] = byte(c.Version >> 8)
	buf[7] = byte(c.Version)
	copy(buf[8:], c.Data)

	_, err := w.Write(buf)
	return err
}
