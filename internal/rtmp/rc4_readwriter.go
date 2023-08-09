package rtmp

import (
	"crypto/rc4"
	"io"
)

type rc4ReadWriter struct {
	rw  io.ReadWriter
	in  *rc4.Cipher
	out *rc4.Cipher
}

func newRC4ReadWriter(rw io.ReadWriter, keyIn []byte, keyOut []byte) (*rc4ReadWriter, error) {
	in, err := rc4.NewCipher(keyIn)
	if err != nil {
		return nil, err
	}

	out, err := rc4.NewCipher(keyOut)
	if err != nil {
		return nil, err
	}

	p := make([]byte, 1536)
	in.XORKeyStream(p, p)
	out.XORKeyStream(p, p)

	return &rc4ReadWriter{
		rw:  rw,
		in:  in,
		out: out,
	}, nil
}

func (r *rc4ReadWriter) Read(p []byte) (int, error) {
	n, err := r.rw.Read(p)
	if n == 0 {
		return 0, err
	}

	r.in.XORKeyStream(p[:n], p[:n])
	return n, err
}

func (r *rc4ReadWriter) Write(p []byte) (int, error) {
	r.out.XORKeyStream(p, p)
	return r.rw.Write(p)
}
