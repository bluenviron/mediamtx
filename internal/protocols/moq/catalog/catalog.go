// Package catalog contains a media catalog.
package catalog

// Catalog is a media catalog.
// spec: draft-ietf-moq-msf-00
type Catalog struct {
	Version int     `json:"version"`
	Tracks  []Track `json:"tracks"`
}

// Track is a track in a media catalog.
type Track struct {
	Name       string  `json:"name"`
	Packaging  string  `json:"packaging"`
	IsLive     bool    `json:"isLive"`
	Namespace  string  `json:"namespace,omitempty"`
	Codec      string  `json:"codec,omitempty"`
	Bitrate    int     `json:"bitrate,omitempty"`
	Width      int     `json:"width,omitempty"`
	Height     int     `json:"height,omitempty"`
	Framerate  float64 `json:"framerate,omitempty"`
	Samplerate int     `json:"samplerate,omitempty"`
	Channels   int     `json:"channels,omitempty"`
	ClockRate  int     `json:"clockrate,omitempty"`
	InitData   string  `json:"initData,omitempty"`
}
