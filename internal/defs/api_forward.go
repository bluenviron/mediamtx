package defs

import (
	"time"

	"github.com/google/uuid"
)

// APIForwardState is the state of a forward.
type APIForwardState string

// forward states.
const (
	APIForwardStateConnecting APIForwardState = "connecting"
	APIForwardStateForwarding APIForwardState = "forwarding"
	APIForwardStateError      APIForwardState = "error"
)

// APIForwardProtocol is the protocol used by a forward.
type APIForwardProtocol string

// forward protocols.
const (
	APIForwardProtocolRTMP  APIForwardProtocol = "rtmp"
	APIForwardProtocolRTMPS APIForwardProtocol = "rtmps"
	APIForwardProtocolRTSP  APIForwardProtocol = "rtsp"
	APIForwardProtocolRTSPS APIForwardProtocol = "rtsps"
	APIForwardProtocolSRT   APIForwardProtocol = "srt"
)

// APIForward is a forward.
type APIForward struct {
	ID            uuid.UUID          `json:"id"`
	Pos           int                `json:"pos"`
	Created       time.Time          `json:"created"`
	Dest          string             `json:"dest"`
	Protocol      APIForwardProtocol `json:"protocol"`
	State         APIForwardState    `json:"state"`
	LastError     string             `json:"lastError"`
	OutboundBytes uint64             `json:"outboundBytes"`
	// deprecated
	BytesSent uint64 `json:"bytesSent" deprecated:"true"`
}

// APIForwardList is a list of forwards.
type APIForwardList struct {
	ItemCount int          `json:"itemCount"`
	PageCount int          `json:"pageCount"`
	Items     []APIForward `json:"items"`
}
