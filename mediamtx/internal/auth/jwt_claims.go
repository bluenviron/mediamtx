package auth

import (
	"encoding/json"
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
	"github.com/golang-jwt/jwt/v5"
)

type jwtClaims struct {
	jwt.RegisteredClaims
	permissionsKey string
	permissions    []conf.AuthInternalUserPermission
}

func (c *jwtClaims) UnmarshalJSON(b []byte) error {
	err := json.Unmarshal(b, &c.RegisteredClaims)
	if err != nil {
		return err
	}

	var claimMap map[string]json.RawMessage
	err = json.Unmarshal(b, &claimMap)
	if err != nil {
		return err
	}

	rawPermissions, ok := claimMap[c.permissionsKey]
	if !ok {
		return fmt.Errorf("claim '%s' not found inside JWT", c.permissionsKey)
	}

	err = jsonwrapper.Unmarshal(rawPermissions, &c.permissions)
	if err != nil {
		var str string
		err = json.Unmarshal(rawPermissions, &str)
		if err != nil {
			return err
		}

		err = jsonwrapper.Unmarshal([]byte(str), &c.permissions)
		if err != nil {
			return err
		}
	}

	return nil
}
