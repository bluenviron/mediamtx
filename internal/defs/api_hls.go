package defs

import (
	"time"

	"github.com/google/uuid"
)

// APIHLSServer contains methods used by the API and Metrics server.
type APIHLSServer interface {
	APISessionsList() (*APIHLSSessionList, error)
	APISessionsGet(uuid.UUID) (*APIHLSSession, error)
	APISessionsKick(uuid.UUID) error
	APIMuxersList() (*APIHLSMuxerList, error)
	APIMuxersGet(string) (*APIHLSMuxer, error)
}

// APIHLSSessionList is a list of HLS sessions.
type APIHLSSessionList struct {
	ItemCount int             `json:"itemCount"`
	PageCount int             `json:"pageCount"`
	Items     []APIHLSSession `json:"items"`
}

// APIHLSSession is an HLS session.
type APIHLSSession struct {
	ID            uuid.UUID `json:"id"`
	Created       time.Time `json:"created"`
	RemoteAddr    string    `json:"remoteAddr"`
	Path          string    `json:"path"`
	Query         string    `json:"query"`
	User          string    `json:"user"`
	OutboundBytes uint64    `json:"outboundBytes"`
}

// APIHLSMuxerList is a list of HLS muxers.
type APIHLSMuxerList struct {
	ItemCount int           `json:"itemCount"`
	PageCount int           `json:"pageCount"`
	Items     []APIHLSMuxer `json:"items"`
}

// APIHLSMuxer is an HLS muxer.
type APIHLSMuxer struct {
	Path                    string    `json:"path"`
	Created                 time.Time `json:"created"`
	LastRequest             time.Time `json:"lastRequest"`
	OutboundBytes           uint64    `json:"outboundBytes"`
	OutboundFramesDiscarded uint64    `json:"outboundFramesDiscarded"`
	// deprecated
	BytesSent uint64 `json:"bytesSent" deprecated:"true"`
}
