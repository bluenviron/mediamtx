package defs

import (
	"time"

	"github.com/google/uuid"
)

// APIPushTargetState is the state of a push target.
type APIPushTargetState string

// push target states.
const (
	APIPushTargetStateConnecting APIPushTargetState = "connecting"
	APIPushTargetStatePushing    APIPushTargetState = "pushing"
	APIPushTargetStateError      APIPushTargetState = "error"
)

// APIPushTargetSource is where a push target was created from.
type APIPushTargetSource string

// push target sources.
const (
	APIPushTargetSourceConfig APIPushTargetSource = "config"
	APIPushTargetSourceAPI    APIPushTargetSource = "api"
)

// APIPushTarget is a push target.
type APIPushTarget struct {
	ID        uuid.UUID           `json:"id"`
	Created   time.Time           `json:"created"`
	URL       string              `json:"url"`
	Source    APIPushTargetSource `json:"source"`
	State     APIPushTargetState  `json:"state"`
	LastError string              `json:"lastError"`
}

// APIPushTargetList is a list of push targets.
type APIPushTargetList struct {
	ItemCount int             `json:"itemCount"`
	PageCount int             `json:"pageCount"`
	Items     []APIPushTarget `json:"items"`
}

// APIPushTargetAdd is a push target add request.
type APIPushTargetAdd struct {
	URL string `json:"url"`
}
