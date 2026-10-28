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
				HasProperties: false,
				IsFirstObject: false,
				TrackAlias:    1,
				GroupID:       0,
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
				HasProperties: true,
				IsFirstObject: false,
				TrackAlias:    1,
				GroupID:       0,
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

func TestHeaderFlags(t *testing.T) {
	for _, header := range [][]byte{
		{0x14, 0x01, 0x00, 0x02, 0x80},
		{0x12, 0x01, 0x00, 0x80},
	} {
		data := append(append([]byte{}, header...), 0x00, 0x01, 'a', 0x00, 0x00, 0x03)
		var decoded subgroup.SubGroup
		require.NoError(t, decoded.Read(bytes.NewReader(data)))
		require.Equal(t, uint64(1), decoded.Header.TrackAlias)
		require.Equal(t, []byte("a"), decoded.Objects[0].Payload)
	}

	for _, flags := range []byte{0x16, 0x90, 0x00, 0x80} {
		var decoded subgroup.SubGroup
		require.Error(t, decoded.Read(bytes.NewReader([]byte{flags, 0x01, 0x00})))
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
