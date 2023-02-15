package core

type EventStream struct {
	Path   string   `json:"path"`
	Ready  bool     `json:"ready"`
	Tracks []string `json:"tracks,omitempty"`
}
