package defs

import (
	"net"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/google/uuid"
)

// PathAccessRequest is a path access request.
type PathAccessRequest struct {
	Name     string
	Query    string
	Publish  bool
	SkipAuth bool

	// only if skipAuth = false
	Proto            auth.Protocol
	ID               *uuid.UUID
	Credentials      *auth.Credentials
	IP               net.IP
	CustomVerifyFunc func(expectedUser string, expectedPass string) bool
}

// ToAuthRequest converts a path access request into an authentication request.
func (r *PathAccessRequest) ToAuthRequest() *auth.Request {
	return &auth.Request{
		Action: func() conf.AuthAction {
			if r.Publish {
				return conf.AuthActionPublish
			}
			return conf.AuthActionRead
		}(),
		Path:             r.Name,
		Query:            r.Query,
		Protocol:         r.Proto,
		ID:               r.ID,
		Credentials:      r.Credentials,
		IP:               r.IP,
		CustomVerifyFunc: r.CustomVerifyFunc,
	}
}
