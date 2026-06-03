package subgroup

import (
	"bytes"
	"testing"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	"github.com/stretchr/testify/require"
)

var cases = []struct {
	name string
	enc  []byte
	dec  SubGroup
}{
	{
		name: "stream without properties",
		enc: []byte{
			0x30,                         // type (Properties=false, FirstObject=false)
			0x01,                         // TrackAlias = 1
			0x00,                         // GroupID = 0
			0x00,                         // object IDDelta = 0
			0x05,                         // payload length = 5
			0x68, 0x65, 0x6c, 0x6c, 0x6f, // payload = "hello"
			0x00, // end-of-stream IDDelta = 0
			0x00, // payload length = 0 (end-of-stream)
			0x03, // status = EndOfGroup
		},
		dec: SubGroup{
			Header: Header{
				Properties:  false,
				FirstObject: false,
				TrackAlias:  1,
				GroupID:     0,
			},
			Objects: []Object{{
				Payload: []byte("hello"),
			}},
		},
	},
	{
		name: "stream with properties",
		enc: []byte{
			0x31,       // type (Properties=true)
			0x01,       // TrackAlias = 1
			0x00,       // GroupID = 0
			0x00,       // object IDDelta = 0
			0x03,       // properties length = 3
			0x06,       // property type delta = 6 (Timestamp)
			0x83, 0xe8, // Timestamp value = 1000
			0x05,                         // payload length = 5
			0x68, 0x65, 0x6c, 0x6c, 0x6f, // payload = "hello"
			0x00, // end-of-stream IDDelta = 0
			0x00, // properties length = 0
			0x00, // payload length = 0 (end-of-stream)
			0x03, // status = EndOfGroup
		},
		dec: SubGroup{
			Header: Header{
				Properties:  true,
				FirstObject: false,
				TrackAlias:  1,
				GroupID:     0,
			},
			Objects: []Object{{
				Properties: property.Properties{
					new(property.Timestamp(1000)),
				},
				Payload: []byte("hello"),
			}},
		},
	},
}

func TestUnmarshal(t *testing.T) {
	for _, ca := range cases {
		t.Run(ca.name, func(t *testing.T) {
			var s SubGroup
			err := s.Read(bytes.NewReader(ca.enc))
			require.NoError(t, err)
			require.Equal(t, ca.dec, s)
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
