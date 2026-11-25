package defs

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrPathReaderNotFound is returned when a path reader is not found.
var ErrPathReaderNotFound = errors.New("path reader not found")

// APIPathManager contains methods used by the API and Metrics server.
type APIPathManager interface {
	APIPathsList() (*APIPathList, error)
	APIPathsGet(string) (*APIPath, error)
	APIReadersList(string) (*APIPathReaderList, error)
	APIReadersGet(string, string) (*APIPathReader, error)
	APIForwardDestsList(string) (*APIForwardDestList, error)
	APIForwardDestsGet(string, uuid.UUID) (*APIForwardDest, error)
	APIStaticSourcesGet(string) (*APIStaticSource, error)
}

// APIPathSourceType is the type of a path source.
type APIPathSourceType string

// source types.
const (
	APIPathSourceTypeHLSSource       APIPathSourceType = "hlsSource"
	APIPathSourceTypeRedirect        APIPathSourceType = "redirect"
	APIPathSourceTypeRPICameraSource APIPathSourceType = "rpiCameraSource"
	APIPathSourceTypeRTMPConn        APIPathSourceType = "rtmpConn"
	APIPathSourceTypeRTMPSConn       APIPathSourceType = "rtmpsConn"
	APIPathSourceTypeRTMPSource      APIPathSourceType = "rtmpSource"
	APIPathSourceTypeRTSPSession     APIPathSourceType = "rtspSession"
	APIPathSourceTypeRTSPSource      APIPathSourceType = "rtspSource"
	APIPathSourceTypeRTSPSSession    APIPathSourceType = "rtspsSession"
	APIPathSourceTypeSRTConn         APIPathSourceType = "srtConn"
	APIPathSourceTypeSRTSource       APIPathSourceType = "srtSource"
	APIPathSourceTypeMPEGTSSource    APIPathSourceType = "mpegtsSource"
	APIPathSourceTypeRTPSource       APIPathSourceType = "rtpSource"
	APIPathSourceTypeWebRTCSession   APIPathSourceType = "webRTCSession"
	APIPathSourceTypeWebRTCSource    APIPathSourceType = "webRTCSource"
	APIPathSourceTypeMoQSource       APIPathSourceType = "moqSource"
	APIPathSourceTypeMoQSession      APIPathSourceType = "moqSession"
)

// APIPathSource is a source.
type APIPathSource struct {
	Type APIPathSourceType `json:"type"`
	ID   string            `json:"id"`
}

// APIPathReaderType is the type of a path reader.
type APIPathReaderType string

// reader types.
const (
	APIPathReaderTypeHLSSession    APIPathReaderType = "hlsSession"
	APIPathReaderTypeRTMPConn      APIPathReaderType = "rtmpConn"
	APIPathReaderTypeRTMPSConn     APIPathReaderType = "rtmpsConn"
	APIPathReaderTypeRTSPConn      APIPathReaderType = "rtspConn"
	APIPathReaderTypeRTSPSession   APIPathReaderType = "rtspSession"
	APIPathReaderTypeRTSPSConn     APIPathReaderType = "rtspsConn"
	APIPathReaderTypeRTSPSSession  APIPathReaderType = "rtspsSession"
	APIPathReaderTypeSRTConn       APIPathReaderType = "srtConn"
	APIPathReaderTypeWebRTCSession APIPathReaderType = "webRTCSession"
	APIPathReaderTypeMoQSession    APIPathReaderType = "moqSession"
	APIPathReaderTypeHidden        APIPathReaderType = "hidden"
)

// APIPathReader is a reader.
type APIPathReader struct {
	Type APIPathReaderType `json:"type"`
	ID   string            `json:"id"`
}

// APIPathReaderList is a list of path readers.
type APIPathReaderList struct {
	ItemCount int             `json:"itemCount"`
	PageCount int             `json:"pageCount"`
	Items     []APIPathReader `json:"items"`
}

// APIPath is a path.
type APIPath struct {
	Name                 string         `json:"name"`
	ConfName             string         `json:"confName"`
	Available            bool           `json:"available"`
	AvailableTime        *time.Time     `json:"availableTime"`
	Online               bool           `json:"online"`
	OnlineTime           *time.Time     `json:"onlineTime"`
	Recording            bool           `json:"recording"`
	Source               *APIPathSource `json:"source"`
	Tracks2              []APIPathTrack `json:"tracks2"`
	ReaderCount          uint64         `json:"readerCount"`
	InboundBytes         uint64         `json:"inboundBytes"`
	OutboundBytes        uint64         `json:"outboundBytes"`
	InboundFramesInError uint64         `json:"inboundFramesInError"`
	ForwardDestCount     uint64         `json:"forwardDestCount"`

	// deprecated
	Ready         bool                `json:"ready" deprecated:"true"`
	ReadyTime     *time.Time          `json:"readyTime" deprecated:"true"`
	BytesReceived uint64              `json:"bytesReceived" deprecated:"true"`
	BytesSent     uint64              `json:"bytesSent" deprecated:"true"`
	Tracks        []APIPathTrackCodec `json:"tracks" deprecated:"true"`
	Readers       []APIPathReader     `json:"readers" deprecated:"true"`
}

// APIPathList is a list of paths.
type APIPathList struct {
	ItemCount int       `json:"itemCount"`
	PageCount int       `json:"pageCount"`
	Items     []APIPath `json:"items"`
}
