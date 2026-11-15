package conf_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
)

func TestRedact(t *testing.T) {
	c := conf.Conf{
		AuthInternalUsers: []conf.AuthInternalUser{
			{
				User: "user1",
				Pass: conf.Credential("pass1"),
			},
			{
				User: "user2",
				Pass: conf.Credential("pass2"),
			},
		},
		PathDefaults: conf.Path{
			PublishPass: func() *conf.Credential {
				v := conf.Credential("publishpass")
				return &v
			}(),
			ReadPass: func() *conf.Credential {
				v := conf.Credential("readpass")
				return &v
			}(),
		},
		Paths: map[string]*conf.Path{
			"path1": {
				PublishPass: func() *conf.Credential {
					v := conf.Credential("path1publishpass")
					return &v
				}(),
				ReadPass: func() *conf.Credential {
					v := conf.Credential("path1readpass")
					return &v
				}(),
			},
			"path2": {
				PublishPass: func() *conf.Credential {
					v := conf.Credential("path2publishpass")
					return &v
				}(),
				ReadPass: func() *conf.Credential {
					v := conf.Credential("path2readpass")
					return &v
				}(),
			},
		},
	}

	c2 := conf.Redact(&c)

	require.Equal(t, "<redacted>", string(c2.AuthInternalUsers[0].Pass))
	require.Equal(t, "<redacted>", string(c2.AuthInternalUsers[1].Pass))
	require.Equal(t, "<redacted>", string(*c2.PathDefaults.PublishPass))
	require.Equal(t, "<redacted>", string(*c2.PathDefaults.ReadPass))
	require.Equal(t, "<redacted>", string(*c2.Paths["path1"].PublishPass))
	require.Equal(t, "<redacted>", string(*c2.Paths["path1"].ReadPass))
	require.Equal(t, "<redacted>", string(*c2.Paths["path2"].PublishPass))
	require.Equal(t, "<redacted>", string(*c2.Paths["path2"].ReadPass))
}

func TestRedactSecrets(t *testing.T) {
	c := conf.Conf{
		HLSCDNSecret: "cdnsecret",
		WebRTCICEServers2: []conf.WebRTCICEServer{{
			URL:      "turn:host:3478",
			Username: "u",
			Password: "turnpass",
		}},
		PathDefaults: conf.Path{
			WHEPBearerToken: "defaulttoken",
		},
		Paths: map[string]*conf.Path{
			"cam": {
				SRTReadPassphrase:    "readpassphrase",
				SRTPublishPassphrase: "publishpassphrase",
				WHEPBearerToken:      "wheptoken",
				Forward: conf.Forward{{
					Dest:            "rtmp://host/app",
					WHIPBearerToken: "whiptoken",
				}},
			},
		},
	}

	c2 := conf.Redact(&c)

	require.Equal(t, "<redacted>", c2.HLSCDNSecret)
	require.Equal(t, "<redacted>", c2.WebRTCICEServers2[0].Password)
	require.Equal(t, "<redacted>", c2.WebRTCICEServers2[0].Username)
	require.Equal(t, "<redacted>", c2.PathDefaults.WHEPBearerToken)

	cam := c2.Paths["cam"]
	require.Equal(t, "<redacted>", cam.SRTReadPassphrase)
	require.Equal(t, "<redacted>", cam.SRTPublishPassphrase)
	require.Equal(t, "<redacted>", cam.WHEPBearerToken)
	require.Equal(t, "<redacted>", cam.Forward[0].WHIPBearerToken)

	require.Equal(t, "cdnsecret", c.HLSCDNSecret)
	require.Equal(t, "wheptoken", c.Paths["cam"].WHEPBearerToken)
}

func TestRedactedWordIsReserved(t *testing.T) {
	for _, ca := range []struct {
		name string
		yml  string
		err  string
	}{
		{
			"pass",
			"authInternalUsers:\n  - user: test\n    pass: <redacted>\n",
			"'pass' cannot be \"<redacted>\", " +
				"this word is reserved to avoid confusion with redacted values returned by the API",
		},
		{
			"hlsCDNSecret",
			"hlsCDNSecret: <redacted>\n",
			"'hlsCDNSecret' cannot be \"<redacted>\", " +
				"this word is reserved to avoid confusion with redacted values returned by the API",
		},
		{
			"webrtcICEServers2 password",
			"webrtcICEServers2:\n  - url: turn:host:3478\n    password: <redacted>\n",
			"'webrtcICEServers2 password' cannot be \"<redacted>\", " +
				"this word is reserved to avoid confusion with redacted values returned by the API",
		},
		{
			"srtReadPassphrase",
			"paths:\n  test:\n    srtReadPassphrase: <redacted>\n",
			"'srtReadPassphrase' cannot be \"<redacted>\", " +
				"this word is reserved to avoid confusion with redacted values returned by the API",
		},
		{
			"srtPublishPassphrase",
			"paths:\n  test:\n    srtPublishPassphrase: <redacted>\n",
			"'srtPublishPassphrase' cannot be \"<redacted>\", " +
				"this word is reserved to avoid confusion with redacted values returned by the API",
		},
		{
			"whepBearerToken",
			"paths:\n  test:\n    whepBearerToken: <redacted>\n",
			"'whepBearerToken' cannot be \"<redacted>\", " +
				"this word is reserved to avoid confusion with redacted values returned by the API",
		},
		{
			"whipBearerToken",
			"paths:\n  test:\n    forward:\n      - dest: rtmp://host/app\n        whipBearerToken: <redacted>\n",
			"invalid 'forward': entry 0: 'whipBearerToken' cannot be \"<redacted>\", " +
				"this word is reserved to avoid confusion with redacted values returned by the API",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			fpath := filepath.Join(t.TempDir(), "mediamtx.yml")
			err := os.WriteFile(fpath, []byte(ca.yml), 0o600)
			require.NoError(t, err)

			_, _, err = conf.Load(fpath, nil, nil)
			require.EqualError(t, err, ca.err)
		})
	}
}

func TestRedactSRTURLs(t *testing.T) {
	c := conf.Conf{
		Paths: map[string]*conf.Path{
			"cam": {
				Source: "srt://host:9000?streamid=read:cam:user:pass&passphrase=secret1234&latency=200",
				Forward: conf.Forward{
					{Dest: "srt://host:9001?passphrase=other1234&streamid=publish:x"},
					{Dest: "rtmp://host/app?passphrase=untouched"},
				},
			},
			"plain": {
				Source: "srt://host:9000?latency=200",
			},
		},
	}

	c2 := conf.Redact(&c)

	cam := c2.Paths["cam"]
	require.Equal(t, "srt://host:9000?streamid=<redacted>&passphrase=<redacted>&latency=200", cam.Source)
	require.Equal(t, "srt://host:9001?passphrase=<redacted>&streamid=<redacted>", cam.Forward[0].Dest)
	require.Equal(t, "rtmp://host/app?passphrase=untouched", cam.Forward[1].Dest)
	require.Equal(t, "srt://host:9000?latency=200", c2.Paths["plain"].Source)

	require.Equal(t, "srt://host:9000?streamid=read:cam:user:pass&passphrase=secret1234&latency=200",
		c.Paths["cam"].Source)
}

func TestRedactURLUserinfo(t *testing.T) {
	c := conf.Conf{
		AuthHTTPAddress: "http://authuser:authpass@auth.example.com/check",
		AuthJWTJWKS:     "https://jwkuser:jwkpass@jwks.example.com/keys?x=a@b",
		Paths: map[string]*conf.Path{
			"cam": {
				Source: "rtsp://user:pa$$word@192.168.1.10:554/stream/$MTX_PATH",
				Forward: conf.Forward{
					{Dest: "rtmp://pub:secret@host/app/$MTX_PATH"},
					{Dest: "srt://host:9001?passphrase=other1234"},
					{Dest: "rtmp://host/app"},
				},
			},
			"emptypass": {Source: "rtsp://user:@host/stream"},
			"nouser":    {Source: "rtsp://host/stream@x"},
			"onlyuser":  {Source: "rtsp://user@host/stream"},
		},
	}

	c2 := conf.Redact(&c)

	require.Equal(t, "http://<redacted>:<redacted>@auth.example.com/check", c2.AuthHTTPAddress)
	require.Equal(t, "https://<redacted>:<redacted>@jwks.example.com/keys?x=a@b", c2.AuthJWTJWKS)

	cam := c2.Paths["cam"]
	require.Equal(t, "rtsp://<redacted>:<redacted>@192.168.1.10:554/stream/$MTX_PATH", cam.Source)
	require.Equal(t, "rtmp://<redacted>:<redacted>@host/app/$MTX_PATH", cam.Forward[0].Dest)
	require.Equal(t, "srt://host:9001?passphrase=<redacted>", cam.Forward[1].Dest)
	require.Equal(t, "rtmp://host/app", cam.Forward[2].Dest)
	require.Equal(t, "rtsp://<redacted>:@host/stream", c2.Paths["emptypass"].Source)
	require.Equal(t, "rtsp://host/stream@x", c2.Paths["nouser"].Source)
	require.Equal(t, "rtsp://<redacted>@host/stream", c2.Paths["onlyuser"].Source)

	require.Equal(t, "rtsp://user:pa$$word@192.168.1.10:554/stream/$MTX_PATH", c.Paths["cam"].Source)
}
