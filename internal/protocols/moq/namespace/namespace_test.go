package namespace_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/namespace"
)

var cases = []struct {
	name string
	enc  []byte
	dec  namespace.Namespace
}{
	{
		name: "empty namespace",
		enc:  []byte{0x00},
		dec:  namespace.Namespace{},
	},
	{
		name: "single part namespace",
		enc: []byte{
			0x01,                   // namespace count = 1
			0x03, 0x66, 0x6f, 0x6f, // namespace[0] = "foo"
		},
		dec: namespace.Namespace{"foo"},
	},
	{
		name: "multiple parts namespace",
		enc: []byte{
			0x03,                   // namespace count = 3
			0x03, 0x66, 0x6f, 0x6f, // namespace[0] = "foo"
			0x03, 0x62, 0x61, 0x72, // namespace[1] = "bar"
			0x03, 0x62, 0x61, 0x7a, // namespace[2] = "baz"
		},
		dec: namespace.Namespace{"foo", "bar", "baz"},
	},
}

func TestUnmarshal(t *testing.T) {
	for _, ca := range cases {
		t.Run(ca.name, func(t *testing.T) {
			var v namespace.Namespace
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
			buf := make([]byte, ca.dec.MarshalSize())
			n := ca.dec.MarshalTo(buf)
			require.Equal(t, len(ca.enc), n)
			require.Equal(t, ca.enc, buf)
		})
	}
}

func FuzzUnmarshal(f *testing.F) {
	for _, ca := range cases {
		f.Add(ca.enc)
	}

	f.Fuzz(func(_ *testing.T, buf []byte) {
		var v namespace.Namespace
		_, err := v.Unmarshal(buf)
		if err != nil {
			return
		}

		out := make([]byte, v.MarshalSize())
		v.MarshalTo(out)
	})
}
