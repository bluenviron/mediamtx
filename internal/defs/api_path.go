package defs

import (
	"time"
)

// APIPathManager contains methods used by the API and Metrics server.
type APIPathManager interface {
	APIPathsList() (*APIPathList, error)
	APIPathsGet(string) (*APIPath, error)
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
	APIPathReaderTypeHLSMuxer           APIPathReaderType = "hlsMuxer"
	APIPathReaderTypeRTMPConn           APIPathReaderType = "rtmpConn"
	APIPathReaderTypeRTMPSConn          APIPathReaderType = "rtmpsConn"
	APIPathReaderTypeRTSPConn           APIPathReaderType = "rtspConn"
	APIPathReaderTypeRPICameraSecondary APIPathReaderType = "rpiCameraSecondary"
	APIPathReaderTypeRTSPSession        APIPathReaderType = "rtspSession"
	APIPathReaderTypeRTSPSConn          APIPathReaderType = "rtspsConn"
	APIPathReaderTypeRTSPSSession       APIPathReaderType = "rtspsSession"
	APIPathReaderTypeSRTConn            APIPathReaderType = "srtConn"
	APIPathReaderTypeWebRTCSession      APIPathReaderType = "webRTCSession"
)

// APIPathReader is a reader.
type APIPathReader struct {
	Type APIPathReaderType `json:"type"`
	ID   string            `json:"id"`
}

// APIPath is a path.
type APIPath struct {
	Name                 string              `json:"name"`
	ConfName             string              `json:"confName"`
	Ready                bool                `json:"ready" deprecated:"true"`
	ReadyTime            *time.Time          `json:"readyTime" deprecated:"true"`
	Available            bool                `json:"available"`
	AvailableTime        *time.Time          `json:"availableTime"`
	Online               bool                `json:"online"`
	OnlineTime           *time.Time          `json:"onlineTime"`
	Source               *APIPathSource      `json:"source"`
	Tracks               []APIPathTrackCodec `json:"tracks" deprecated:"true"`
	Tracks2              []APIPathTrack      `json:"tracks2"`
	Readers              []APIPathReader     `json:"readers"`
	InboundBytes         uint64              `json:"inboundBytes"`
	OutboundBytes        uint64              `json:"outboundBytes"`
	InboundFramesInError uint64              `json:"inboundFramesInError"`
	// deprecated
	BytesReceived uint64 `json:"bytesReceived" deprecated:"true"`
	BytesSent     uint64 `json:"bytesSent" deprecated:"true"`
}

// APIPathList is a list of paths.
type APIPathList struct {
	ItemCount int       `json:"itemCount"`
	PageCount int       `json:"pageCount"`
	Items     []APIPath `json:"items"`
}
