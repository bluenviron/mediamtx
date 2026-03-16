package defs

import (
	"time"

	"github.com/google/uuid"
)

// APIWebRTCServer contains methods used by the API and Metrics server.
type APIWebRTCServer interface {
	APISessionsList() (*APIWebRTCSessionList, error)
	APISessionsGet(uuid.UUID) (*APIWebRTCSession, error)
	APISessionsKick(uuid.UUID) error
}

// APIWebRTCSessionState is the state of a WebRTC connection.
type APIWebRTCSessionState string

// states.
const (
	APIWebRTCSessionStateRead    APIWebRTCSessionState = "read"
	APIWebRTCSessionStatePublish APIWebRTCSessionState = "publish"
)

// APIWebRTCSession is a WebRTC session.
type APIWebRTCSession struct {
	ID                        uuid.UUID             `json:"id"`
	Created                   time.Time             `json:"created"`
	RemoteAddr                string                `json:"remoteAddr"`
	PeerConnectionEstablished bool                  `json:"peerConnectionEstablished"`
	LocalCandidate            string                `json:"localCandidate"`
	RemoteCandidate           string                `json:"remoteCandidate"`
	State                     APIWebRTCSessionState `json:"state"`
	Path                      string                `json:"path"`
	Query                     string                `json:"query"`
	User                      string                `json:"user"`
	InboundBytes              uint64                `json:"inboundBytes"`
	InboundRTPPackets         uint64                `json:"inboundRTPPackets"`
	InboundRTPPacketsLost     uint64                `json:"inboundRTPPacketsLost"`
	InboundRTPPacketsJitter   float64               `json:"inboundRTPPacketsJitter"`
	InboundRTCPPackets        uint64                `json:"inboundRTCPPackets"`
	OutboundBytes             uint64                `json:"outboundBytes"`
	OutboundRTPPackets        uint64                `json:"outboundRTPPackets"`
	OutboundRTCPPackets       uint64                `json:"outboundRTCPPackets"`
	// deprecated
	BytesReceived       uint64  `json:"bytesReceived" deprecated:"true"`
	BytesSent           uint64  `json:"bytesSent" deprecated:"true"`
	RTPPacketsReceived  uint64  `json:"rtpPacketsReceived" deprecated:"true"`
	RTPPacketsSent      uint64  `json:"rtpPacketsSent" deprecated:"true"`
	RTPPacketsLost      uint64  `json:"rtpPacketsLost" deprecated:"true"`
	RTPPacketsJitter    float64 `json:"rtpPacketsJitter" deprecated:"true"`
	RTCPPacketsReceived uint64  `json:"rtcpPacketsReceived" deprecated:"true"`
	RTCPPacketsSent     uint64  `json:"rtcpPacketsSent" deprecated:"true"`
}

// APIWebRTCSessionList is a list of WebRTC sessions.
type APIWebRTCSessionList struct {
	ItemCount int                `json:"itemCount"`
	PageCount int                `json:"pageCount"`
	Items     []APIWebRTCSession `json:"items"`
}
