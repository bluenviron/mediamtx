package test

import "github.com/bluenviron/mediamtx/internal/auth"

// AuthManager is a test auth manager.
type AuthManager struct {
	Func func(req *auth.Request) error
}

// Authenticate replicates auth.Manager.Replicate
func (m *AuthManager) Authenticate(req *auth.Request) error {
	return m.Func(req)
}

// NilAuthManager is an auth manager that accepts everything.
var NilAuthManager = &AuthManager{
	Func: func(_ *auth.Request) error {
		return nil
	},
}
