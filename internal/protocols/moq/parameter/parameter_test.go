package parameter_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/version"
)

var cases = []struct {
	name  string
	count int
	enc   []byte
	dec   parameter.Parameters
}{
	{
		name:  "no parameters",
		count: 0,
		enc:   []byte{},
		dec:   nil,
	},
	{
		name:  "authorization token",
		count: 1,
		enc: []byte{
			0x03,                               // type delta = 3 (AuthorizationToken)
			0x08,                               // inner length = 8
			0x03,                               // alias type = UseValue
			0x01,                               // token type = 1
			0x73, 0x65, 0x63, 0x72, 0x65, 0x74, // token value = "secret"
		},
		dec: parameter.Parameters{
			&parameter.AuthorizationToken{
				AliasType:  parameter.AuthorizationTokenAliasTypeUseValue,
				TokenType:  1,
				TokenValue: []byte("secret"),
			},
		},
	},
}

func TestUnmarshal(t *testing.T) {
	for _, ca := range cases {
		t.Run(ca.name, func(t *testing.T) {
			var params parameter.Parameters
			_, err := params.Unmarshal(ca.count, ca.enc, version.Draft19)
			require.NoError(t, err)
			require.Equal(t, ca.dec, params)
		})
	}
}

func TestMarshal(t *testing.T) {
	for _, ca := range cases {
		t.Run(ca.name, func(t *testing.T) {
			buf := make([]byte, ca.dec.MarshalSize())
			ca.dec.MarshalTo(buf, version.Draft19)
			require.Equal(t, ca.enc, buf)
		})
	}
}

func TestIncludeProperties(t *testing.T) {
	for _, versio := range []version.Version{version.Draft19, version.Draft20, version.Draft21, version.Draft22} {
		var params parameter.Parameters
		n, err := params.Unmarshal(1, []byte{0x35, 0x00}, versio)
		require.NoError(t, err)
		require.Equal(t, 2, n)
		require.Equal(t, parameter.Parameters{&parameter.IncludeProperties{Value: false}}, params)

		buf := make([]byte, params.MarshalSize())
		params.MarshalTo(buf, versio)
		require.Equal(t, []byte{0x35, 0x00}, buf)
	}

	var params parameter.Parameters
	_, err := params.Unmarshal(1, []byte{0x35, 0x02}, version.Draft22)
	require.Error(t, err)
}

func TestLocationFilterVersions(t *testing.T) {
	for _, ca := range []struct {
		version version.Version
		encoded []byte
		filter  *parameter.LocationFilter
	}{
		{version.Draft20, []byte{0x21, 0x02, 0x00, 0x00}, &parameter.LocationFilter{Fields: []uint64{0, 0}}},
		{version.Draft21, []byte{0x21, 0x02, 0x00, 0x00}, &parameter.LocationFilter{Fields: []uint64{0, 0}}},
		{version.Draft22, []byte{0x21, 0x05}, &parameter.LocationFilter{Type: 5}},
	} {
		var params parameter.Parameters
		n, err := params.Unmarshal(1, ca.encoded, ca.version)
		require.NoError(t, err)
		require.Equal(t, len(ca.encoded), n)
		require.Equal(t, ca.filter, params[0])
	}

	for _, encoded := range [][]byte{{0x21, 0x02, 0x00}, {0x21, 0x05, 0x00}} {
		var params parameter.Parameters
		_, err := params.Unmarshal(1, encoded, version.Draft20)
		require.Error(t, err)
	}
	var params parameter.Parameters
	_, err := params.Unmarshal(1, []byte{0x21, 0x06}, version.Draft22)
	require.Error(t, err)
}

func FuzzUnmarshal(f *testing.F) {
	for _, ca := range cases {
		f.Add(ca.count, ca.enc)
	}

	f.Fuzz(func(_ *testing.T, count int, buf []byte) {
		var params parameter.Parameters
		_, err := params.Unmarshal(count, buf, version.Draft19)
		if err != nil {
			return
		}

		buf = make([]byte, params.MarshalSize())
		params.MarshalTo(buf, version.Draft19)
	})
}
