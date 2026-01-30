package conf

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// AuthMethod is an authentication method.
type AuthMethod string

// authentication methods.
const (
	AuthMethodInternal AuthMethod = "internal"
	AuthMethodHTTP     AuthMethod = "http"
	AuthMethodJWT      AuthMethod = "jwt"
)

// UnmarshalJSON implements json.Unmarshaler.
func (d *AuthMethod) UnmarshalJSON(b []byte) error {
	type alias AuthMethod
	if err := jsonwrapper.Unmarshal(b, (*alias)(d)); err != nil {
		return err
	}

	switch *d {
	case AuthMethodInternal, AuthMethodHTTP, AuthMethodJWT:

	default:
		return fmt.Errorf("invalid authMethod: '%s'", *d)
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *AuthMethod) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}
