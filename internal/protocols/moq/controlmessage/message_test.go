package controlmessage

import (
	"bytes"
	"testing"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/stretchr/testify/require"
)

var cases = []struct {
	name string
	enc  []byte
	dec  Message
}{
	{
		name: "setup",
		enc: []byte{
			0xAF, 0x00, // type 0x2F00 (2-byte varint)
			0x00, 0x00, // length = 0
		},
		dec: &Setup{},
	},
	{
		name: "subscribe",
		enc: []byte{
			0x03,       // type 0x03
			0x00, 0x0B, // length = 11
			0x01,                   // RequestID = 1
			0x01,                   // namespace count = 1
			0x03, 0x66, 0x6F, 0x6F, // namespace[0] = "foo"
			0x03, 0x62, 0x61, 0x72, // track name = "bar"
			0x00, // parameters count = 0
		},
		dec: &Subscribe{
			RequestID: 1,
			Namespace: []string{"foo"},
			TrackName: "bar",
		},
	},
	{
		name: "subscribe with params",
		enc: []byte{
			0x03,       // type 0x03
			0x00, 0x15, // length = 21
			0x01,                   // RequestID = 1
			0x01,                   // namespace count = 1
			0x03, 0x66, 0x6F, 0x6F, // namespace[0] = "foo"
			0x03, 0x62, 0x61, 0x72, // track name = "bar"
			0x01,                               // parameters count = 1
			0x03,                               // type delta = 3 (AuthorizationToken)
			0x08,                               // inner length = 8
			0x03,                               // alias type = UseValue
			0x01,                               // token type = 1
			0x73, 0x65, 0x63, 0x72, 0x65, 0x74, // token value = "secret"
		},
		dec: &Subscribe{
			RequestID: 1,
			Namespace: []string{"foo"},
			TrackName: "bar",
			Parameters: []parameter.Parameter{
				&parameter.AuthorizationToken{
					AliasType:  parameter.AuthorizationTokenAliasTypeUseValue,
					TokenType:  1,
					TokenValue: []byte("secret"),
				},
			},
		},
	},
	{
		name: "subscribe_ok",
		enc: []byte{
			0x04,       // type 0x04
			0x00, 0x02, // length = 2
			0x01, // TrackAlias = 1
			0x00, // Number of Parameters = 0
		},
		dec: &SubscribeOk{
			TrackAlias: 1,
		},
	},
	{
		name: "request_error",
		enc: []byte{
			0x05,       // type 0x05
			0x00, 0x06, // length = 6
			0x01,                   // Code = 1
			0x00,                   // retryInterval = 0 (ignored)
			0x03, 0x66, 0x6F, 0x6F, // Reason = "foo"
		},
		dec: &RequestError{
			Code:   1,
			Reason: "foo",
		},
	},
	{
		name: "request_ok",
		enc: []byte{
			0x07,       // type 0x07
			0x00, 0x01, // length = 1
			0x00, // Number of Parameters = 0
		},
		dec: &RequestOk{},
	},
	{
		name: "publish",
		enc: []byte{
			0x1D,       // type 0x1D
			0x00, 0x0C, // length = 12
			0x01,                   // RequestID = 1
			0x01,                   // namespace count = 1
			0x03, 0x66, 0x6F, 0x6F, // namespace[0] = "foo"
			0x03, 0x62, 0x61, 0x72, // track name = "bar"
			0x02, // TrackAlias = 2
			0x00, // parameters count = 0
		},
		dec: &Publish{
			RequestID:  1,
			Namespace:  []string{"foo"},
			TrackName:  "bar",
			TrackAlias: 2,
		},
	},
	{
		name: "publish with parameters",
		enc: []byte{
			0x1D,       // type 0x1D
			0x00, 0x16, // length = 22
			0x01,                   // RequestID = 1
			0x01,                   // namespace count = 1
			0x03, 0x66, 0x6F, 0x6F, // namespace[0] = "foo"
			0x03, 0x62, 0x61, 0x72, // track name = "bar"
			0x02,                               // TrackAlias = 2
			0x01,                               // parameters count = 1
			0x03,                               // type delta = 3 (AuthorizationToken)
			0x08,                               // inner length = 8
			0x03,                               // alias type = UseValue
			0x01,                               // token type = 1
			0x73, 0x65, 0x63, 0x72, 0x65, 0x74, // token value = "secret"
		},
		dec: &Publish{
			RequestID:  1,
			Namespace:  []string{"foo"},
			TrackName:  "bar",
			TrackAlias: 2,
			Parameters: []parameter.Parameter{
				&parameter.AuthorizationToken{
					AliasType:  parameter.AuthorizationTokenAliasTypeUseValue,
					TokenType:  1,
					TokenValue: []byte("secret"),
				},
			},
		},
	},
}

func TestUnmarshal(t *testing.T) {
	for _, ca := range cases {
		t.Run(ca.name, func(t *testing.T) {
			m, err := Read(bytes.NewReader(ca.enc))
			require.NoError(t, err)
			require.Equal(t, ca.dec, m)
		})
	}
}

func TestMarshal(t *testing.T) {
	for _, ca := range cases {
		t.Run(ca.name, func(t *testing.T) {
			buf := ca.dec.Marshal()
			require.Equal(t, ca.enc, buf)
		})
	}
}

func FuzzUnmarshal(f *testing.F) {
	for _, ca := range cases {
		f.Add(ca.enc)
	}

	f.Fuzz(func(_ *testing.T, buf []byte) {
		m, err := Read(bytes.NewReader(buf))
		if err != nil {
			return
		}

		m.Marshal()
	})
}
