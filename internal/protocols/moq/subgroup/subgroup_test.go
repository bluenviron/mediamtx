package subgroup_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
)

var cases = []struct {
	name string
	enc  []byte
	dec  subgroup.SubGroup
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
		dec: subgroup.SubGroup{
			Header: subgroup.Header{
				Properties:  false,
				FirstObject: false,
				TrackAlias:  1,
				GroupID:     0,
			},
			Objects: []subgroup.Object{{
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
		dec: subgroup.SubGroup{
			Header: subgroup.Header{
				Properties:  true,
				FirstObject: false,
				TrackAlias:  1,
				GroupID:     0,
			},
			Objects: []subgroup.Object{{
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
			var s subgroup.SubGroup
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

func FuzzUnmarshal(f *testing.F) {
	for _, ca := range cases {
		f.Add(ca.enc)
	}

	f.Fuzz(func(t *testing.T, buf []byte) {
		var s subgroup.SubGroup
		err := s.Read(bytes.NewReader(buf))
		if err != nil {
			return
		}

		require.NotEmpty(t, s.Objects)

		for _, obj := range s.Objects {
			require.NotEmpty(t, obj.Payload)
		}

		s.Marshal()
	})
}
