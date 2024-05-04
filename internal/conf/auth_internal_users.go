package conf

import (
	"encoding/json"
)

// AuthInternalUserPermission is a permission of a user.
type AuthInternalUserPermission struct {
	Action AuthAction `json:"action"`
	Path   string     `json:"path"`
}

// AuthInternalUser is an user.
type AuthInternalUser struct {
	User        Credential                   `json:"user"`
	Pass        Credential                   `json:"pass"`
	IPs         IPNetworks                   `json:"ips"`
	Permissions []AuthInternalUserPermission `json:"permissions"`
}

// AuthInternalUsers is a list of AuthInternalUser
type AuthInternalUsers []AuthInternalUser

// UnmarshalJSON implements json.Unmarshaler.
func (s *AuthInternalUsers) UnmarshalJSON(b []byte) error {
	// remove default value before loading new value
	// https://github.com/golang/go/issues/21092
	*s = nil
	return json.Unmarshal(b, (*[]AuthInternalUser)(s))
}

// AuthInternalUserPermissions is a list of AuthInternalUserPermission
type AuthInternalUserPermissions []AuthInternalUserPermission

// UnmarshalJSON implements json.Unmarshaler.
func (s *AuthInternalUserPermissions) UnmarshalJSON(b []byte) error {
	// remove default value before loading new value
	// https://github.com/golang/go/issues/21092
	*s = nil
	return json.Unmarshal(b, (*[]AuthInternalUserPermission)(s))
}
