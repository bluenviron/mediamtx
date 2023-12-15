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
type RPICamera struct {
	Params Params
	OnData func(time.Duration, [][]byte)
}

// Initialize initializes a RPICamera.
func (c *RPICamera) Initialize() error {
	return fmt.Errorf("server was compiled without support for the Raspberry Pi Camera")
}

// Close closes a RPICamera.
func (c *RPICamera) Close() {
}

// ReloadParams reloads the camera parameters.
func (c *RPICamera) ReloadParams(_ Params) {
}
