package defs

import (
	"time"
)

// APIStaticSourceState is the state of a static source.
type APIStaticSourceState string

// static source states.
const (
	APIStaticSourceStateIdle    APIStaticSourceState = "idle"
	APIStaticSourceStateRunning APIStaticSourceState = "running"
	APIStaticSourceStateError   APIStaticSourceState = "error"
)

// APIStaticSource is a static source.
type APIStaticSource struct {
	Type      APIPathSourceType    `json:"type"`
	State     APIStaticSourceState `json:"state"`
	LastError string               `json:"lastError"`
	Created   time.Time            `json:"created"`
}
