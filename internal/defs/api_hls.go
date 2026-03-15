package defs

import "time"

// APIHLSServer contains methods used by the API and Metrics server.
type APIHLSServer interface {
	APIMuxersList() (*APIHLSMuxerList, error)
	APIMuxersGet(string) (*APIHLSMuxer, error)
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
	ItemCount int           `json:"itemCount"`
	PageCount int           `json:"pageCount"`
	Items     []APIHLSMuxer `json:"items"`
}
