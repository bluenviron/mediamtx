package varint

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

var cases = []struct {
	name string
	enc  []byte
	dec  Varint
}{
	{
		name: "1 byte",
		enc:  []byte{0x00},
		dec:  0,
	},
	{
		name: "2 bytes",
		enc:  []byte{0x83, 0xe8},
		dec:  1000,
	},
	{
		name: "3 bytes",
		enc:  []byte{0xc0, 0x40, 0x00},
		dec:  16384,
	},
	{
		name: "4 bytes",
		enc:  []byte{0xe0, 0x20, 0x00, 0x00},
		dec:  2097152,
	},
	{
		name: "5 bytes",
		enc:  []byte{0xf0, 0x10, 0x00, 0x00, 0x00},
		dec:  268435456,
	},
	{
		name: "6 bytes",
		enc:  []byte{0xf8, 0x08, 0x00, 0x00, 0x00, 0x00},
		dec:  34359738368,
	},
	{
		name: "7 bytes",
		enc:  []byte{0xfc, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00},
		dec:  4398046511104,
	},
	{
		name: "8 bytes",
		enc:  []byte{0xfe, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		dec:  562949953421312,
	},
	{
		name: "9 bytes",
		enc:  []byte{0xff, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		dec:  72057594037927936,
	},
}

func TestRead(t *testing.T) {
	for _, ca := range cases {
		t.Run(ca.name, func(t *testing.T) {
			var v Varint
			err := v.Read(bytes.NewReader(ca.enc))
			require.NoError(t, err)
			require.Equal(t, ca.dec, v)
		})
	}
}

func TestUnmarshal(t *testing.T) {
	for _, ca := range cases {
		t.Run(ca.name, func(t *testing.T) {
			var v Varint
			n, err := v.Unmarshal(ca.enc)
			require.NoError(t, err)
			require.Equal(t, len(ca.enc), n)
			require.Equal(t, ca.dec, v)
		})
	}
}

func TestMarshal(t *testing.T) {
	for _, ca := range cases {
		t.Run(ca.name, func(t *testing.T) {
			require.Equal(t, ca.enc, ca.dec.Marshal())
		})
	}
}

func FuzzUnmarshal(f *testing.F) {
	for _, ca := range cases {
		f.Add(ca.enc)
	}

	f.Fuzz(func(_ *testing.T, buf []byte) {
		var v Varint
		_, err := v.Unmarshal(buf)
		if err != nil {
			return
		}

		v.Marshal()
	})
}

func FuzzRead(f *testing.F) {
	for _, ca := range cases {
		f.Add(ca.enc)
	}

	f.Fuzz(func(_ *testing.T, buf []byte) {
		var v Varint
		err := v.Read(bytes.NewReader(buf))
		if err != nil {
			return
		}

		v.Marshal()
	})
}
