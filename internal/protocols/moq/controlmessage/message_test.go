package controlmessage_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/namespace"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/version"
)

var cases = []struct {
	name string
	enc  []byte
	dec  controlmessage.Message
}{
	{
		name: "draft-16 client setup",
		enc: []byte{
			0x20,       // type 0x20
			0x00, 0x00, // length = 0
		},
		dec: &controlmessage.ClientSetup{},
	},
	{
		name: "draft-16 server setup",
		enc: []byte{
			0x21,       // type 0x21
			0x00, 0x00, // length = 0
		},
		dec: &controlmessage.ServerSetup{},
	},
	{
		name: "setup",
		enc: []byte{
			0xAF, 0x00, // type 0x2F00 (2-byte varint)
			0x00, 0x00, // length = 0
		},
		dec: &controlmessage.Setup{},
	},
	{
		name: "setup with path",
		enc: []byte{
			0xAF, 0x00, // type 0x2F00 (2-byte varint)
			0x00, 0x06, // length = 6
			0x01,                   // delta type = PATH (0x01)
			0x04,                   // option len
			0x2F, 0x66, 0x6F, 0x6F, // "/foo"
		},
		dec: &controlmessage.Setup{
			Path: "/foo",
		},
	},
	{
		name: "setup with path and authority",
		enc: []byte{
			0xAF, 0x00, // type 0x2F00 (2-byte varint)
			0x00, 0x11, // length = 17
			0x01,                   // delta type = PATH (0x01 - 0 = 0x01)
			0x04,                   // option len
			0x2F, 0x66, 0x6F, 0x6F, // "/foo"
			0x04,                                                 // delta type = AUTHORITY (0x05 - 0x01 = 0x04)
			0x09,                                                 // option len
			0x6C, 0x6F, 0x63, 0x61, 0x6C, 0x68, 0x6F, 0x73, 0x74, // "localhost"
		},
		dec: &controlmessage.Setup{
			Path:      "/foo",
			Authority: "localhost",
		},
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
		dec: &controlmessage.Subscribe{
			RequestID: 1,
			Namespace: namespace.Namespace{"foo"},
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
		dec: &controlmessage.Subscribe{
			RequestID: 1,
			Namespace: namespace.Namespace{"foo"},
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
		dec: &controlmessage.SubscribeOk{
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
		dec: &controlmessage.RequestError{
			Code:   1,
			Reason: "foo",
		},
	},
	{
		name: "publish_ok",
		enc: []byte{
			0x1E,       // type 0x1E
			0x00, 0x01, // length = 1
			0x00, // Number of Parameters = 0
		},
		dec: &controlmessage.PublishOk{},
	},
	{
		name: "request_ok",
		enc: []byte{
			0x07,       // type 0x07
			0x00, 0x01, // length = 1
			0x00, // Number of Parameters = 0
		},
		dec: &controlmessage.RequestOk{},
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
		dec: &controlmessage.Publish{
			RequestID:  1,
			Namespace:  namespace.Namespace{"foo"},
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
		dec: &controlmessage.Publish{
			RequestID:  1,
			Namespace:  namespace.Namespace{"foo"},
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
			m, err := controlmessage.Read(bytes.NewReader(ca.enc), version.Draft19)
			require.NoError(t, err)
			require.Equal(t, ca.dec, m)
			require.NotEmpty(t, m.Marshal(version.Draft19))
		})
	}
}

func TestPublishStateNotify(t *testing.T) {
	encoded := []byte{0x22, 0x00, 0x01, 0x00}
	for _, versio := range []version.Version{version.Draft19, version.Draft20, version.Draft21, version.Draft22} {
		msg, err := controlmessage.Read(bytes.NewReader(encoded), versio)
		require.NoError(t, err)
		require.Equal(t, &controlmessage.PublishStateNotify{}, msg)
		require.Equal(t, encoded, msg.Marshal(versio))
	}
}

func TestVersionedSubscribeParameters(t *testing.T) {
	encoded := []byte{0x03, 0x00, 0x06, 0x01, 0x00, 0x00, 0x01, 0x35, 0x00}
	for _, versio := range []version.Version{version.Draft19, version.Draft20, version.Draft21, version.Draft22} {
		msg, err := controlmessage.Read(bytes.NewReader(encoded), versio)
		require.NoError(t, err)
		require.Equal(t, &controlmessage.Subscribe{
			RequestID:  1,
			Namespace:  namespace.Namespace{},
			Parameters: parameter.Parameters{&parameter.IncludeProperties{Value: false}},
		}, msg)
	}
	encoded[2]++
	_, err := controlmessage.Read(bytes.NewReader(append(encoded, 0x00)), version.Draft22)
	require.Error(t, err)
}

func TestMarshal(t *testing.T) {
	for _, ca := range cases {
		t.Run(ca.name, func(t *testing.T) {
			buf := ca.dec.Marshal(version.Draft19)
			require.Equal(t, ca.enc, buf)
		})
	}
}

func TestString(t *testing.T) {
	for _, ca := range []struct {
		message controlmessage.Message
		want    string
	}{
		{&controlmessage.Setup{Path: "/video", Authority: "example.com"}, "SETUP path=/video authority=example.com"},
		{&controlmessage.ClientSetup{Path: "/video"}, "CLIENT_SETUP path=/video authority="},
		{&controlmessage.ServerSetup{}, "SERVER_SETUP path= authority="},
		{
			&controlmessage.Subscribe{RequestID: 1, Namespace: namespace.Namespace{"news"}, TrackName: "audio"},
			"SUBSCRIBE requestID=1 namespace=[news] trackName=audio",
		},
		{&controlmessage.SubscribeOk{TrackAlias: 2}, "SUBSCRIBE_OK trackAlias=2"},
		{
			&controlmessage.RequestError{Code: controlmessage.RequestErrorCodeNotSupported, Reason: "unavailable"},
			"REQUEST_ERROR code=3 reason=unavailable",
		},
		{
			&controlmessage.Publish{RequestID: 3, Namespace: namespace.Namespace{"news"}, TrackName: "video", TrackAlias: 4},
			"PUBLISH requestID=3 namespace=[news] trackName=video trackAlias=4",
		},
		{&controlmessage.PublishOk{}, "PUBLISH_OK"},
		{&controlmessage.RequestOk{}, "REQUEST_OK"},
		{&controlmessage.PublishStateNotify{}, "PUBLISH_STATE_NOTIFY"},
	} {
		require.Equal(t, ca.want, ca.message.String())
	}
}

func FuzzUnmarshal(f *testing.F) {
	for _, ca := range cases {
		f.Add(ca.enc)
	}

	f.Fuzz(func(_ *testing.T, buf []byte) {
		m, err := controlmessage.Read(bytes.NewReader(buf), version.Draft19)
		if err != nil {
			return
		}

		m.Marshal(version.Draft19)
	})
}
