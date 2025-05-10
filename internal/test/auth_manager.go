package test

import "github.com/bluenviron/mediamtx/internal/auth"

// AuthManager is a dummy auth manager.
type AuthManager struct {
	fnc func(req *auth.Request) error
	forceRefreshFnc func() error
}

// Authenticate replicates auth.Manager.Replicate
func (m *AuthManager) Authenticate(req *auth.Request) error {
	return m.fnc(req)
}

func (m *AuthManager) ForceRefreshJWTJWKS() error {
	return m.forceRefreshFnc()
}

// NilAuthManager is an auth manager that accepts everything.
var NilAuthManager = &AuthManager{
	fnc: func(_ *auth.Request) error {
		return nil
	},
	forceRefreshFnc: func() error {
		return nil
	},
}
