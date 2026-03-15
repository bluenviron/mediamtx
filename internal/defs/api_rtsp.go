package defs

import (
	"time"

	"github.com/google/uuid"
)

// APIRTSPServer contains methods used by the API and Metrics server.
type APIRTSPServer interface {
	APIConnsList() (*APIRTSPConnsList, error)
	APIConnsGet(uuid.UUID) (*APIRTSPConn, error)
	APISessionsList() (*APIRTSPSessionList, error)
	APISessionsGet(uuid.UUID) (*APIRTSPSession, error)
	APISessionsKick(uuid.UUID) error
}

// APIRTSPConn is a RTSP connection.
type APIRTSPConn struct {
	ID            uuid.UUID  `json:"id"`
	Created       time.Time  `json:"created"`
	RemoteAddr    string     `json:"remoteAddr"`
	Session       *uuid.UUID `json:"session"`
	Tunnel        string     `json:"tunnel"`
	BytesReceived uint64     `json:"bytesReceived"`
	BytesSent     uint64     `json:"bytesSent"`
}

// APIRTSPConnsList is a list of RTSP connections.
type APIRTSPConnsList struct {
	ItemCount int           `json:"itemCount"`
	PageCount int           `json:"pageCount"`
	Items     []APIRTSPConn `json:"items"`
}

// APIRTSPSessionState is the state of a RTSP session.
type APIRTSPSessionState string

// states.
const (
	APIRTSPSessionStateIdle    APIRTSPSessionState = "idle"
	APIRTSPSessionStateRead    APIRTSPSessionState = "read"
	APIRTSPSessionStatePublish APIRTSPSessionState = "publish"
)

// APIRTSPSession is a RTSP session.
type APIRTSPSession struct {
	ID                  uuid.UUID           `json:"id"`
	Created             time.Time           `json:"created"`
	RemoteAddr          string              `json:"remoteAddr"`
	State               APIRTSPSessionState `json:"state"`
	Path                string              `json:"path"`
	Query               string              `json:"query"`
	User                string              `json:"user"`
	Transport           *string             `json:"transport"`
	Profile             *string             `json:"profile"`
	Conns               []uuid.UUID         `json:"conns"`
	BytesReceived       uint64              `json:"bytesReceived"`
	BytesSent           uint64              `json:"bytesSent"`
	RTPPacketsReceived  uint64              `json:"rtpPacketsReceived"`
	RTPPacketsSent      uint64              `json:"rtpPacketsSent"`
	RTPPacketsLost      uint64              `json:"rtpPacketsLost"`
	RTPPacketsInError   uint64              `json:"rtpPacketsInError"`
	RTPPacketsJitter    float64             `json:"rtpPacketsJitter"`
	RTCPPacketsReceived uint64              `json:"rtcpPacketsReceived"`
	RTCPPacketsSent     uint64              `json:"rtcpPacketsSent"`
	RTCPPacketsInError  uint64              `json:"rtcpPacketsInError"`
}

// APIRTSPSessionList is a list of RTSP sessions.
type APIRTSPSessionList struct {
	ItemCount int              `json:"itemCount"`
	PageCount int              `json:"pageCount"`
	Items     []APIRTSPSession `json:"items"`
}
