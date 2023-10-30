//go:build !rpicamera
// +build !rpicamera

// Package rpicamera allows to interact with a Raspberry Pi Camera.
package rpicamera

import (
	"fmt"
	"time"
)

// Cleanup cleanups files created by the camera implementation.
func Cleanup() {
}

// RPICamera is a RPI Camera reader.
type RPICamera struct{}

// New allocates a RPICamera.
func New(
	_ Params,
	_ func(time.Duration, [][]byte),
) (*RPICamera, error) {
	return nil, fmt.Errorf("server was compiled without support for the Raspberry Pi Camera")
}

// Close closes a RPICamera.
func (c *RPICamera) Close() {
}

// ReloadParams reloads the camera parameters.
func (c *RPICamera) ReloadParams(_ Params) {
}
