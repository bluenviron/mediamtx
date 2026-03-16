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
	InboundBytes  uint64     `json:"inboundBytes"`
	OutboundBytes uint64     `json:"outboundBytes"`
	BytesReceived uint64     `json:"bytesReceived" deprecated:"true"`
	BytesSent     uint64     `json:"bytesSent" deprecated:"true"`
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
	ID                             uuid.UUID           `json:"id"`
	Created                        time.Time           `json:"created"`
	RemoteAddr                     string              `json:"remoteAddr"`
	State                          APIRTSPSessionState `json:"state"`
	Path                           string              `json:"path"`
	Query                          string              `json:"query"`
	User                           string              `json:"user"`
	Transport                      *string             `json:"transport"`
	Profile                        *string             `json:"profile"`
	Conns                          []uuid.UUID         `json:"conns"`
	InboundBytes                   uint64              `json:"inboundBytes"`
	InboundRTPPackets              uint64              `json:"inboundRTPPackets"`
	InboundRTPPacketsLost          uint64              `json:"inboundRTPPacketsLost"`
	InboundRTPPacketsInError       uint64              `json:"inboundRTPPacketsInError"`
	InboundRTPPacketsJitter        float64             `json:"inboundRTPPacketsJitter"`
	InboundRTCPPackets             uint64              `json:"inboundRTCPPackets"`
	InboundRTCPPacketsInError      uint64              `json:"inboundRTCPPacketsInError"`
	OutboundBytes                  uint64              `json:"outboundBytes"`
	OutboundRTPPackets             uint64              `json:"outboundRTPPackets"`
	OutboundRTPPacketsReportedLost uint64              `json:"outboundRTPPacketsReportedLost"`
	OutboundRTCPPackets            uint64              `json:"outboundRTCPPackets"`
	// deprecated
	BytesReceived       uint64  `json:"bytesReceived" deprecated:"true"`
	BytesSent           uint64  `json:"bytesSent" deprecated:"true"`
	RTPPacketsReceived  uint64  `json:"rtpPacketsReceived" deprecated:"true"`
	RTPPacketsSent      uint64  `json:"rtpPacketsSent" deprecated:"true"`
	RTPPacketsLost      uint64  `json:"rtpPacketsLost" deprecated:"true"`
	RTPPacketsInError   uint64  `json:"rtpPacketsInError" deprecated:"true"`
	RTPPacketsJitter    float64 `json:"rtpPacketsJitter" deprecated:"true"`
	RTCPPacketsReceived uint64  `json:"rtcpPacketsReceived" deprecated:"true"`
	RTCPPacketsSent     uint64  `json:"rtcpPacketsSent" deprecated:"true"`
	RTCPPacketsInError  uint64  `json:"rtcpPacketsInError" deprecated:"true"`
}

// APIRTSPSessionList is a list of RTSP sessions.
type APIRTSPSessionList struct {
	ItemCount int              `json:"itemCount"`
	PageCount int              `json:"pageCount"`
	Items     []APIRTSPSession `json:"items"`
}
