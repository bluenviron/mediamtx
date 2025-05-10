package rtsp

import (
	"testing"

	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/stretchr/testify/require"
)

func TestCredentials(t *testing.T) {
	rr := &base.Request{
		Header: base.Header{
			"Authorization": []string{
				"Basic bXl1c2VyOm15cGFzcw==",
			},
		},
	}

	c := Credentials(rr)

	require.Equal(t, &auth.Credentials{
		User: "myuser",
		Pass: "mypass",
	}, c)
}
