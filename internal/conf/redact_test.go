package conf_test

import (
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
