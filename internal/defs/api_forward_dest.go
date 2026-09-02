package defs

import (
	"time"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
)

// APIForwardDestState is the state of a forward destination.
type APIForwardDestState string

// forward destination states.
const (
	APIForwardDestStateIdle       APIForwardDestState = "idle"
	APIForwardDestStateForwarding APIForwardDestState = "forwarding"
	APIForwardDestStateError      APIForwardDestState = "error"
)

// APIForwardDestType is the type of a forward destination.
type APIForwardDestType string

// forward destination types.
const (
	APIForwardDestTypeRTMP   APIForwardDestType = "rtmp"
	APIForwardDestTypeRTSP   APIForwardDestType = "rtsp"
	APIForwardDestTypeSRT    APIForwardDestType = "srt"
	APIForwardDestTypeMoQ    APIForwardDestType = "moq"
	APIForwardDestTypeWebRTC APIForwardDestType = "webRTC"
)

// APIForwardDestProtocol is the protocol used by a forward destination.
//
// Deprecated: replaced by APIForwardDestType.
type APIForwardDestProtocol string

// forward destination protocols.
const (
	APIForwardDestProtocolRTMP  APIForwardDestProtocol = "rtmp"
	APIForwardDestProtocolRTMPS APIForwardDestProtocol = "rtmps"
	APIForwardDestProtocolRTSP  APIForwardDestProtocol = "rtsp"
	APIForwardDestProtocolRTSPS APIForwardDestProtocol = "rtsps"
	APIForwardDestProtocolSRT   APIForwardDestProtocol = "srt"
	APIForwardDestProtocolMoQ   APIForwardDestProtocol = "moq"
	APIForwardDestProtocolWHIP  APIForwardDestProtocol = "whip"
	APIForwardDestProtocolWHIPS APIForwardDestProtocol = "whips"
)

// APIForwardDest is a forward destination.
type APIForwardDest struct {
	ID            uuid.UUID           `json:"id"`
	Pos           int                 `json:"pos"`
	Created       time.Time           `json:"created"`
	Conf          conf.ForwardDest    `json:"conf"`
	Type          APIForwardDestType  `json:"type"`
	State         APIForwardDestState `json:"state"`
	LastError     string              `json:"lastError"`
	OutboundBytes uint64              `json:"outboundBytes"`

	// deprecated
	Protocol APIForwardDestProtocol `json:"protocol" deprecated:"true"`
}

// APIForwardDestList is a list of forward destinations.
type APIForwardDestList struct {
	ItemCount int              `json:"itemCount"`
	PageCount int              `json:"pageCount"`
	Items     []APIForwardDest `json:"items"`
}
