// Package handshake contains the RTMP handshake mechanism.
package handshake

import (
	"io"
)

// DoClient performs a client-side handshake.
func DoClient(rw io.ReadWriter, validateSignature bool) error {
	c0 := C0S0{}
	err := c0.Write(rw)
	if err != nil {
		return err
	}

	c1 := C1S1{}
	err = c1.Write(rw, true)
	if err != nil {
		return err
	}

	s0 := C0S0{}
	err = s0.Read(rw)
	if err != nil {
		return err
	}

	s1 := C1S1{}
	err = s1.Read(rw, false, validateSignature)
	if err != nil {
		return err
	}

	s2 := C2S2{
		Digest: c1.Digest,
	}
	err = s2.Read(rw, validateSignature)
	if err != nil {
		return err
	}

	c2 := C2S2{
		Time:   s1.Time,
		Random: s1.Random,
		Digest: s1.Digest,
	}
	err = c2.Write(rw)
	if err != nil {
		return err
	}

	return nil
}

// DoServer performs a server-side handshake.
func DoServer(rw io.ReadWriter, validateSignature bool) error {
	err := C0S0{}.Read(rw)
	if err != nil {
		return err
	}

	c1 := C1S1{}
	err = c1.Read(rw, true, validateSignature)
	if err != nil {
		return err
	}

	s0 := C0S0{}
	err = s0.Write(rw)
	if err != nil {
		return err
	}

	s1 := C1S1{}
	err = s1.Write(rw, false)
	if err != nil {
		return err
	}

	s2 := C2S2{
		Time:   c1.Time,
		Random: c1.Random,
		Digest: c1.Digest,
	}
	err = s2.Write(rw)
	if err != nil {
		return err
	}

	c2 := C2S2{Digest: s1.Digest}
	err = c2.Read(rw, validateSignature)
	if err != nil {
		return err
	}

	return nil
}
