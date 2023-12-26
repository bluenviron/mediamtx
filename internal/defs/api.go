package defs

import (
	"time"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
)

// APIError is a generic error.
type APIError struct {
	Error string `json:"error"`
}

// APIPathConfList is a list of path configurations.
type APIPathConfList struct {
	ItemCount int          `json:"itemCount"`
	PageCount int          `json:"pageCount"`
	Items     []*conf.Path `json:"items"`
}

// APIPathSourceOrReader is a source or a reader.
type APIPathSourceOrReader struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// APIPath is a path.
type APIPath struct {
	Name          string                  `json:"name"`
	ConfName      string                  `json:"confName"`
	Source        *APIPathSourceOrReader  `json:"source"`
	Ready         bool                    `json:"ready"`
	ReadyTime     *time.Time              `json:"readyTime"`
	Tracks        []string                `json:"tracks"`
	BytesReceived uint64                  `json:"bytesReceived"`
	BytesSent     uint64                  `json:"bytesSent"`
	Readers       []APIPathSourceOrReader `json:"readers"`
}

// APIPathList is a list of paths.
type APIPathList struct {
	ItemCount int        `json:"itemCount"`
	PageCount int        `json:"pageCount"`
	Items     []*APIPath `json:"items"`
}

// APIHLSMuxer is an HLS muxer.
type APIHLSMuxer struct {
	Path        string    `json:"path"`
	Created     time.Time `json:"created"`
	LastRequest time.Time `json:"lastRequest"`
	BytesSent   uint64    `json:"bytesSent"`
}

// APIHLSMuxerList is a list of HLS muxers.
type APIHLSMuxerList struct {
	ItemCount int            `json:"itemCount"`
	PageCount int            `json:"pageCount"`
	Items     []*APIHLSMuxer `json:"items"`
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
	ID            uuid.UUID        `json:"id"`
	Created       time.Time        `json:"created"`
	RemoteAddr    string           `json:"remoteAddr"`
	State         APIRTMPConnState `json:"state"`
	Path          string           `json:"path"`
	Query         string           `json:"query"`
	BytesReceived uint64           `json:"bytesReceived"`
	BytesSent     uint64           `json:"bytesSent"`
}

// APIRTMPConnList is a list of RTMP connections.
type APIRTMPConnList struct {
	ItemCount int            `json:"itemCount"`
	PageCount int            `json:"pageCount"`
	Items     []*APIRTMPConn `json:"items"`
}

// APIRTSPConn is a RTSP connection.
type APIRTSPConn struct {
	ID            uuid.UUID `json:"id"`
	Created       time.Time `json:"created"`
	RemoteAddr    string    `json:"remoteAddr"`
	BytesReceived uint64    `json:"bytesReceived"`
	BytesSent     uint64    `json:"bytesSent"`
}

// APIRTSPConnsList is a list of RTSP connections.
type APIRTSPConnsList struct {
	ItemCount int            `json:"itemCount"`
	PageCount int            `json:"pageCount"`
	Items     []*APIRTSPConn `json:"items"`
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
	ID            uuid.UUID           `json:"id"`
	Created       time.Time           `json:"created"`
	RemoteAddr    string              `json:"remoteAddr"`
	State         APIRTSPSessionState `json:"state"`
	Path          string              `json:"path"`
	Query         string              `json:"query"`
	Transport     *string             `json:"transport"`
	BytesReceived uint64              `json:"bytesReceived"`
	BytesSent     uint64              `json:"bytesSent"`
}

// APIRTSPSessionList is a list of RTSP sessions.
type APIRTSPSessionList struct {
	ItemCount int               `json:"itemCount"`
	PageCount int               `json:"pageCount"`
	Items     []*APIRTSPSession `json:"items"`
}

// APISRTConnState is the state of a SRT connection.
type APISRTConnState string

// states.
const (
	APISRTConnStateIdle    APISRTConnState = "idle"
	APISRTConnStateRead    APISRTConnState = "read"
	APISRTConnStatePublish APISRTConnState = "publish"
)

// APISRTConn is a SRT connection.
type APISRTConn struct {
	ID            uuid.UUID       `json:"id"`
	Created       time.Time       `json:"created"`
	RemoteAddr    string          `json:"remoteAddr"`
	State         APISRTConnState `json:"state"`
	Path          string          `json:"path"`
	Query         string          `json:"query"`
	BytesReceived uint64          `json:"bytesReceived"`
	BytesSent     uint64          `json:"bytesSent"`
}

// APISRTConnList is a list of SRT connections.
type APISRTConnList struct {
	ItemCount int           `json:"itemCount"`
	PageCount int           `json:"pageCount"`
	Items     []*APISRTConn `json:"items"`
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
	BytesReceived             uint64                `json:"bytesReceived"`
	BytesSent                 uint64                `json:"bytesSent"`
}

// APIWebRTCSessionList is a list of WebRTC sessions.
type APIWebRTCSessionList struct {
	ItemCount int                 `json:"itemCount"`
	PageCount int                 `json:"pageCount"`
	Items     []*APIWebRTCSession `json:"items"`
}
