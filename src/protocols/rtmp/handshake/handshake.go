// Package handshake contains the RTMP handshake mechanism.
package handshake

import (
	"bytes"
	"fmt"
	"io"
)

const (
	encryptedVersion = 3<<24 | 5<<16 | 1<<8 | 1
)

func doClientEncrypted(rw io.ReadWriter) ([]byte, []byte, error) {
	var c0 C0S0

	c0.Version = 6

	err := c0.Write(rw)
	if err != nil {
		return nil, nil, err
	}

	localPrivateKey, localPublicKey, err := dhGenerateKeyPair()
	if err != nil {
		return nil, nil, err
	}

	var c1 C1S1

	c1Digest, err := c1.fill(false, localPublicKey)
	if err != nil {
		return nil, nil, err
	}

	err = c1.Write(rw)
	if err != nil {
		return nil, nil, err
	}

	var s0 C0S0

	err = s0.Read(rw)
	if err != nil {
		return nil, nil, err
	}

	if s0.Version != 6 {
		return nil, nil, fmt.Errorf("server replied with unexpected version %d", s0.Version)
	}

	var s1 C1S1

	err = s1.Read(rw)
	if err != nil {
		return nil, nil, err
	}

	s1Digest, remotePublicKey, err := s1.validate(true)
	if err != nil {
		return nil, nil, err
	}

	var s2 C2S2

	err = s2.Read(rw)
	if err != nil {
		return nil, nil, err
	}

	err = s2.validate(true, c1Digest)
	if err != nil {
		return nil, nil, err
	}

	var c2 C2S2

	err = c2.fill(false, s1Digest)
	if err != nil {
		return nil, nil, err
	}

	err = c2.Write(rw)
	if err != nil {
		return nil, nil, err
	}

	sharedSecret := dhComputeSharedSecret(localPrivateKey, remotePublicKey)
	keyIn := hmacSha256(sharedSecret, localPublicKey)[:16]
	keyOut := hmacSha256(sharedSecret, remotePublicKey)[:16]
	return keyIn, keyOut, nil
}

func doClientPlain(rw io.ReadWriter, strict bool) error {
	var c0 C0S0

	c0.Version = 3

	err := c0.Write(rw)
	if err != nil {
		return err
	}

	var c1 C1S1

	err = c1.fillPlain()
	if err != nil {
		return err
	}

	err = c1.Write(rw)
	if err != nil {
		return err
	}

	var s0 C0S0

	err = s0.Read(rw)
	if err != nil {
		return err
	}

	if s0.Version != 3 {
		return fmt.Errorf("server replied with unexpected version %d", s0.Version)
	}

	var s1 C1S1

	err = s1.Read(rw)
	if err != nil {
		return err
	}

	var s2 C2S2

	err = s2.Read(rw)
	if err != nil {
		return err
	}

	if strict && !bytes.Equal(s2.Data, c1.Data) {
		return fmt.Errorf("data in S2 does not correspond")
	}

	var c2 C2S2

	c2.Data = s1.Data

	return c2.Write(rw)
}

// DoClient performs a client-side handshake.
func DoClient(rw io.ReadWriter, encrypted bool, strict bool) ([]byte, []byte, error) {
	if encrypted {
		return doClientEncrypted(rw)
	}
	return nil, nil, doClientPlain(rw, strict)
}

func doServerEncrypted(rw io.ReadWriter) ([]byte, []byte, error) {
	var c1 C1S1

	err := c1.Read(rw)
	if err != nil {
		return nil, nil, err
	}

	c1Digest, remotePublicKey, err := c1.validate(false)
	if err != nil {
		return nil, nil, err
	}

	localPrivateKey, localPublicKey, err := dhGenerateKeyPair()
	if err != nil {
		return nil, nil, err
	}

	var s0 C0S0

	s0.Version = 6

	err = s0.Write(rw)
	if err != nil {
		return nil, nil, err
	}

	var s1 C1S1

	s1.Version = encryptedVersion

	s1Digest, err := s1.fill(true, localPublicKey)
	if err != nil {
		return nil, nil, err
	}

	err = s1.Write(rw)
	if err != nil {
		return nil, nil, err
	}

	var s2 C2S2

	s2.Time2 = encryptedVersion

	err = s2.fill(true, c1Digest)
	if err != nil {
		return nil, nil, err
	}

	err = s2.Write(rw)
	if err != nil {
		return nil, nil, err
	}

	var c2 C2S2

	err = c2.Read(rw)
	if err != nil {
		return nil, nil, err
	}

	err = c2.validate(false, s1Digest)
	if err != nil {
		return nil, nil, err
	}

	sharedSecret := dhComputeSharedSecret(localPrivateKey, remotePublicKey)
	keyIn := hmacSha256(sharedSecret, localPublicKey)[:16]
	keyOut := hmacSha256(sharedSecret, remotePublicKey)[:16]
	return keyIn, keyOut, nil
}

func doServerPlain(rw io.ReadWriter, strict bool) error {
	var c1 C1S1

	err := c1.Read(rw)
	if err != nil {
		return err
	}

	var s0 C0S0

	s0.Version = 3

	err = s0.Write(rw)
	if err != nil {
		return err
	}

	var s1 C1S1

	err = s1.fillPlain()
	if err != nil {
		return err
	}

	err = s1.Write(rw)
	if err != nil {
		return err
	}

	var s2 C2S2

	s2.Data = c1.Data

	err = s2.Write(rw)
	if err != nil {
		return err
	}

	var c2 C2S2

	err = c2.Read(rw)
	if err != nil {
		return err
	}

	if strict && !bytes.Equal(c2.Data, s1.Data) {
		return fmt.Errorf("data in C2 does not correspond")
	}

	return nil
}

// DoServer performs a server-side handshake.
func DoServer(rw io.ReadWriter, strict bool) ([]byte, []byte, error) {
	var c0 C0S0

	err := c0.Read(rw)
	if err != nil {
		return nil, nil, err
	}

	if c0.Version == 6 {
		return doServerEncrypted(rw)
	}
	return nil, nil, doServerPlain(rw, strict)
}
