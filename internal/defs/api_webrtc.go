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
	BytesReceived             uint64                `json:"bytesReceived"`
	BytesSent                 uint64                `json:"bytesSent"`
	RTPPacketsReceived        uint64                `json:"rtpPacketsReceived"`
	RTPPacketsSent            uint64                `json:"rtpPacketsSent"`
	RTPPacketsLost            uint64                `json:"rtpPacketsLost"`
	RTPPacketsJitter          float64               `json:"rtpPacketsJitter"`
	RTCPPacketsReceived       uint64                `json:"rtcpPacketsReceived"`
	RTCPPacketsSent           uint64                `json:"rtcpPacketsSent"`
}

// APIWebRTCSessionList is a list of WebRTC sessions.
type APIWebRTCSessionList struct {
	ItemCount int                `json:"itemCount"`
	PageCount int                `json:"pageCount"`
	Items     []APIWebRTCSession `json:"items"`
}
