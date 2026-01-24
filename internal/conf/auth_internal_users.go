package conf

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

// AuthInternalUsers is a list of AuthInternalUser.
type AuthInternalUsers []AuthInternalUser

// AuthInternalUserPermissions is a list of AuthInternalUserPermission.
type AuthInternalUserPermissions []AuthInternalUserPermission
