package defs

import (
	"time"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
)

// APIForwardDestState is the state of a forward destination.
type APIForwardDestState string

// forward states.
const (
	APIForwardDestStateIdle       APIForwardDestState = "idle"
	APIForwardDestStateForwarding APIForwardDestState = "forwarding"
	APIForwardDestStateError      APIForwardDestState = "error"
)

// APIForwardDestProtocol is the protocol used by a forward destination.
type APIForwardDestProtocol string

// forward protocols.
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
	ID            uuid.UUID              `json:"id"`
	Pos           int                    `json:"pos"`
	Created       time.Time              `json:"created"`
	Conf          conf.ForwardDest       `json:"conf"`
	Protocol      APIForwardDestProtocol `json:"protocol"`
	State         APIForwardDestState    `json:"state"`
	LastError     string                 `json:"lastError"`
	OutboundBytes uint64                 `json:"outboundBytes"`
}

// APIForwardDestList is a list of forward destinations.
type APIForwardDestList struct {
	ItemCount int              `json:"itemCount"`
	PageCount int              `json:"pageCount"`
	Items     []APIForwardDest `json:"items"`
}
