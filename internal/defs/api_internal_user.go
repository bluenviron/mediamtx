package defs

import (
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
)

// APIInternalUser is an internal user configuration with its runtime identity.
type APIInternalUser struct {
	ID          uuid.UUID                         `json:"id"`
	Pos         int                               `json:"pos"`
	User        conf.Credential                   `json:"user"`
	Pass        conf.Credential                   `json:"pass"`
	IPs         conf.IPNetworks                   `json:"ips"`
	Permissions []conf.AuthInternalUserPermission `json:"permissions"`
}

// APIInternalUserList is a list of internal user configurations.
type APIInternalUserList struct {
	ItemCount int               `json:"itemCount"`
	PageCount int               `json:"pageCount"`
	Items     []APIInternalUser `json:"items"`
}

// APIInternalUserAddRes is returned when an internal user is added.
type APIInternalUserAddRes struct {
	Status APIOKStatus `json:"status"`
	ID     uuid.UUID   `json:"id"`
}
