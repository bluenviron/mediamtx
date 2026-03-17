package defs

import (
	"time"

	"github.com/google/uuid"
)

// APIRTMPServer contains methods used by the API and Metrics server.
type APIRTMPServer interface {
	APIConnsList() (*APIRTMPConnList, error)
	APIConnsGet(uuid.UUID) (*APIRTMPConn, error)
	APIConnsKick(uuid.UUID) error
}

// APIRTMPConnState is the state of a RTMP connection.
type APIRTMPConnState string

// states.
const (
	APIRTMPConnStateIdle    APIRTMPConnState = "idle"
	APIRTMPConnStateRead    APIRTMPConnState = "read"
	APIRTMPConnStatePublish APIRTMPConnState = "publish"
)

// APIRTMPConn is a RTMP connection.
type APIRTMPConn struct {
	ID                      uuid.UUID        `json:"id"`
	Created                 time.Time        `json:"created"`
	RemoteAddr              string           `json:"remoteAddr"`
	State                   APIRTMPConnState `json:"state"`
	Path                    string           `json:"path"`
	Query                   string           `json:"query"`
	User                    string           `json:"user"`
	InboundBytes            uint64           `json:"inboundBytes"`
	OutboundBytes           uint64           `json:"outboundBytes"`
	OutboundFramesDiscarded uint64           `json:"outboundFramesDiscarded"`
	// deprecated
	BytesReceived uint64 `json:"bytesReceived" deprecated:"true"`
	BytesSent     uint64 `json:"bytesSent" deprecated:"true"`
}

// APIRTMPConnList is a list of RTMP connections.
type APIRTMPConnList struct {
	ItemCount int           `json:"itemCount"`
	PageCount int           `json:"pageCount"`
	Items     []APIRTMPConn `json:"items"`
}
