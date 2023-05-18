package core

import (
	"time"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
)

type apiPath struct {
	Name          string         `json:"name"`
	ConfName      string         `json:"confName"`
	Conf          *conf.PathConf `json:"conf"`
	Source        interface{}    `json:"source"`
	SourceReady   bool           `json:"sourceReady"`
	Tracks        []string       `json:"tracks"`
	BytesReceived uint64         `json:"bytesReceived"`
	Readers       []interface{}  `json:"readers"`
}

type apiPathsList struct {
	ItemCount int        `json:"itemCount"`
	PageCount int        `json:"pageCount"`
	Items     []*apiPath `json:"items"`
}

type apiHLSMuxer struct {
	Path        string    `json:"path"`
	Created     time.Time `json:"created"`
	LastRequest time.Time `json:"lastRequest"`
	BytesSent   uint64    `json:"bytesSent"`
}

type apiHLSMuxersList struct {
	ItemCount int            `json:"itemCount"`
	PageCount int            `json:"pageCount"`
	Items     []*apiHLSMuxer `json:"items"`
}

type apiRTSPConn struct {
	ID            uuid.UUID `json:"id"`
	Created       time.Time `json:"created"`
	RemoteAddr    string    `json:"remoteAddr"`
	BytesReceived uint64    `json:"bytesReceived"`
	BytesSent     uint64    `json:"bytesSent"`
}

type apiRTSPConnsList struct {
	ItemCount int            `json:"itemCount"`
	PageCount int            `json:"pageCount"`
	Items     []*apiRTSPConn `json:"items"`
}

type apiRTMPConn struct {
	ID            uuid.UUID `json:"id"`
	Created       time.Time `json:"created"`
	RemoteAddr    string    `json:"remoteAddr"`
	State         string    `json:"state"`
	BytesReceived uint64    `json:"bytesReceived"`
	BytesSent     uint64    `json:"bytesSent"`
}

type apiRTMPConnsList struct {
	ItemCount int            `json:"itemCount"`
	PageCount int            `json:"pageCount"`
	Items     []*apiRTMPConn `json:"items"`
}

type apiRTSPSession struct {
	ID            uuid.UUID `json:"id"`
	Created       time.Time `json:"created"`
	RemoteAddr    string    `json:"remoteAddr"`
	State         string    `json:"state"`
	BytesReceived uint64    `json:"bytesReceived"`
	BytesSent     uint64    `json:"bytesSent"`
}

type apiRTSPSessionsList struct {
	ItemCount int               `json:"itemCount"`
	PageCount int               `json:"pageCount"`
	Items     []*apiRTSPSession `json:"items"`
}

type apiWebRTCSession struct {
	ID                        uuid.UUID `json:"id"`
	Created                   time.Time `json:"created"`
	RemoteAddr                string    `json:"remoteAddr"`
	PeerConnectionEstablished bool      `json:"peerConnectionEstablished"`
	LocalCandidate            string    `json:"localCandidate"`
	RemoteCandidate           string    `json:"remoteCandidate"`
	State                     string    `json:"state"`
	BytesReceived             uint64    `json:"bytesReceived"`
	BytesSent                 uint64    `json:"bytesSent"`
}

type apiWebRTCSessionsList struct {
	ItemCount int                 `json:"itemCount"`
	PageCount int                 `json:"pageCount"`
	Items     []*apiWebRTCSession `json:"items"`
}
