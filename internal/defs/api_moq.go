package defs

import (
	"time"

	"github.com/google/uuid"
)

// APIMoQServer contains methods used by the API server.
type APIMoQServer interface {
	APISessionsList() (*APIMoQSessionList, error)
	APISessionsGet(uuid.UUID) (*APIMoQSession, error)
	APISessionsKick(uuid.UUID) error
}

// APIMoQSessionState is the state of a MoQ session.
type APIMoQSessionState string

// states.
const (
	APIMoQSessionStateIdle    APIMoQSessionState = "idle"
	APIMoQSessionStateRead    APIMoQSessionState = "read"
	APIMoQSessionStatePublish APIMoQSessionState = "publish"
)

// APIMoQVersion is a MoQ protocol version.
type APIMoQVersion string

// protocol versions.
const (
	APIMoQVersionDraft18 APIMoQVersion = "moqt-18"
	APIMoQVersionDraft19 APIMoQVersion = "moqt-19"
)

// APIMoQSessionList is a list of MoQ sessions.
type APIMoQSessionList struct {
	ItemCount int             `json:"itemCount"`
	PageCount int             `json:"pageCount"`
	Items     []APIMoQSession `json:"items"`
}

// APIMoQSession is a MoQ session.
type APIMoQSession struct {
	ID            uuid.UUID          `json:"id"`
	Created       time.Time          `json:"created"`
	RemoteAddr    string             `json:"remoteAddr"`
	State         APIMoQSessionState `json:"state"`
	Path          string             `json:"path"`
	Query         string             `json:"query"`
	UserAgent     string             `json:"userAgent"`
	Version       APIMoQVersion      `json:"version"`
	InboundBytes  uint64             `json:"inboundBytes"`
	OutboundBytes uint64             `json:"outboundBytes"`
}
