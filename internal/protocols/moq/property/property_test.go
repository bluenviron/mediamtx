package property_test

import (
	"testing"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	"github.com/stretchr/testify/require"
)

var cases = []struct {
	name string
	enc  []byte
	dec  property.Properties
}{
	{
		name: "no properties",
		enc:  []byte{},
		dec:  nil,
	},
	{
		name: "timestamp",
		enc: []byte{
			0x06,       // type delta = 6 (Timestamp)
			0x83, 0xe8, // value = 1000
		},
		dec: property.Properties{
			new(property.Timestamp(1000)),
		},
	},
}

func TestUnmarshal(t *testing.T) {
	for _, ca := range cases {
		t.Run(ca.name, func(t *testing.T) {
			var props property.Properties
			err := props.Unmarshal(ca.enc)
			require.NoError(t, err)
			require.Equal(t, ca.dec, props)
		})
	}
}

func TestMarshal(t *testing.T) {
	for _, ca := range cases {
		t.Run(ca.name, func(t *testing.T) {
			buf := make([]byte, ca.dec.MarshalSize())
			ca.dec.MarshalTo(buf)
			require.Equal(t, ca.enc, buf)
		})
	}
}

func FuzzUnmarshal(f *testing.F) {
	for _, ca := range cases {
		f.Add(ca.enc)
	}

	f.Fuzz(func(_ *testing.T, buf []byte) {
		var props property.Properties
		err := props.Unmarshal(buf)
		if err != nil {
			return
		}

		buf = make([]byte, props.MarshalSize())
		props.MarshalTo(buf)
	})
}
