package rtsp

import (
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/headers"
	"github.com/bluenviron/mediamtx/internal/auth"
)

// Credentials extracts credentials from a RTSP request.
func Credentials(rt *base.Request) *auth.Credentials {
	c := &auth.Credentials{}

	var rtspAuthHeader headers.Authorization
	err := rtspAuthHeader.Unmarshal(rt.Header["Authorization"])
	if err == nil {
		c.User = rtspAuthHeader.Username
		if rtspAuthHeader.Method == headers.AuthMethodBasic {
			c.Pass = rtspAuthHeader.BasicPass
		}
	}

	return c
}
