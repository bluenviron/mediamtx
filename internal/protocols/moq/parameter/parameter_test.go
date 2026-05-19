package parameter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var cases = []struct {
	name  string
	count int
	enc   []byte
	dec   Parameters
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
		dec: Parameters{
			&AuthorizationToken{
				AliasType:  AuthorizationTokenAliasTypeUseValue,
				TokenType:  1,
				TokenValue: []byte("secret"),
			},
		},
	},
}

func TestUnmarshal(t *testing.T) {
	for _, ca := range cases {
		t.Run(ca.name, func(t *testing.T) {
			var params Parameters
			_, err := params.Unmarshal(ca.count, ca.enc)
			require.NoError(t, err)
			require.Equal(t, ca.dec, params)
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
