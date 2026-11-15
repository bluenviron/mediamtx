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
	require.Equal(t, "u", c2.WebRTCICEServers2[0].Username)
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
