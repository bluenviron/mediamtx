package conf

// AuthInternalUser is an user.
type AuthInternalUser struct {
	User          Credential                   `json:"user"`
	Pass          Credential                   `json:"pass"`
	IPs           IPNetworks                   `json:"ips"`
	SRTPassphrase string                       `json:"srtPassphrase"`
	Permissions   []AuthInternalUserPermission `json:"permissions"`
}
