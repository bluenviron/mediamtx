package conf

// AuthInternalUserPermission is a permission of a user.
type AuthInternalUserPermission struct {
	Action AuthAction `json:"action"`
	Path   string     `json:"path"`
}
