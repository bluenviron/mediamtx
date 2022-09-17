// Package handshake contains the RTMP handshake mechanism.
package handshake

import (
	"io"
)

// DoClient performs a client-side handshake.
func DoClient(rw io.ReadWriter, validateSignature bool) error {
	err := C0S0{}.Write(rw)
	if err != nil {
		return err
	}

	c1 := C1S1{}
	err = c1.Write(rw, true)
	if err != nil {
		return err
	}

	err = C0S0{}.Read(rw)
	if err != nil {
		return err
	}

	s1 := C1S1{}
	err = s1.Read(rw, false, validateSignature)
	if err != nil {
		return err
	}

	err = (&C2S2{Digest: c1.Digest}).Read(rw, validateSignature)
	if err != nil {
		return err
	}

	err = C2S2{Digest: s1.Digest}.Write(rw)
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

	err = C0S0{}.Write(rw)
	if err != nil {
		return err
	}

	s1 := C1S1{}
	err = s1.Write(rw, false)
	if err != nil {
		return err
	}

	err = C2S2{Digest: c1.Digest}.Write(rw)
	if err != nil {
		return err
	}

	err = (&C2S2{Digest: s1.Digest}).Read(rw, validateSignature)
	if err != nil {
		return err
	}

	return nil
}
