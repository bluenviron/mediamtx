package test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/auth"
)

func TestAuthManager(t *testing.T) {
	m := &AuthManager{
		AuthenticateImpl: func(_ *auth.Request) (string, *auth.Error) {
			return "myuser", nil
		},
	}

	res, err := m.Authenticate(&auth.Request{})
	require.Nil(t, err)
	require.Equal(t, auth.Result{User: "myuser"}, res)
}
