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

// APIForwardSource is where a forward was created from.
type APIForwardSource string

// forward sources.
const (
	APIForwardSourceConfig APIForwardSource = "config"
	APIForwardSourceAPI    APIForwardSource = "api"
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
	Created       time.Time          `json:"created"`
	Dest          string             `json:"dest"`
	Protocol      APIForwardProtocol `json:"protocol"`
	Source        APIForwardSource   `json:"source"`
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

// APIForwardAdd is a forward add request.
type APIForwardAdd struct {
	Dest string `json:"dest"`
}
