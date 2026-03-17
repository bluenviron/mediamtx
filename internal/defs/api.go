package defs

import (
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
)

// APIOKStatus is the status of a successful response.
type APIOKStatus string

// statuses.
const (
	APIOKStatusOK APIOKStatus = "ok"
)

// APIErrorStatus is the status of an error response.
type APIErrorStatus string

// statuses.
const (
	APIErrorStatusError APIErrorStatus = "error"
)

// APIOK is returned on success.
type APIOK struct {
	Status APIOKStatus `json:"status"`
}

// APIError is a generic error.
type APIError struct {
	Status APIErrorStatus `json:"status"`
	Error  string         `json:"error"`
}

// APIInfo is a info response.
type APIInfo struct {
	Version string    `json:"version"`
	Started time.Time `json:"started"`
}

// APIPathConfList is a list of path configurations.
type APIPathConfList struct {
	ItemCount int         `json:"itemCount"`
	PageCount int         `json:"pageCount"`
	Items     []conf.Path `json:"items"`
}
