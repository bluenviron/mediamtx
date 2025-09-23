// Package test contains test utilities.
package test

import "github.com/bluenviron/mediamtx/internal/auth"

// AuthManager is a dummy auth manager.
type AuthManager struct {
	AuthenticateImpl   func(req *auth.Request) *auth.Error
	RefreshJWTJWKSImpl func()
}

// Authenticate replicates auth.Manager.Replicate
func (m *AuthManager) Authenticate(req *auth.Request) *auth.Error {
	return m.AuthenticateImpl(req)
}

// RefreshJWTJWKS is a function that simulates a JWKS refresh.
func (m *AuthManager) RefreshJWTJWKS() {
	m.RefreshJWTJWKSImpl()
}

// NilAuthManager is an auth manager that accepts everything.
var NilAuthManager = &AuthManager{
	AuthenticateImpl: func(_ *auth.Request) *auth.Error {
		return nil
	},
}
