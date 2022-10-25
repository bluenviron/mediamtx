//go:build !rpicamera
// +build !rpicamera

// Package rpicamera allows to interact with a Raspberry Pi Camera.
package rpicamera

import (
	"fmt"
	"time"
)

// RPICamera is a RPI Camera reader.
type RPICamera struct{}

// New allocates a RPICamera.
func New(
	params Params,
	onData func(time.Duration, [][]byte),
) (*RPICamera, error) {
	return nil, fmt.Errorf("server was compiled without support for the Raspberry Pi Camera")
}

// Close closes a RPICamera.
func (c *RPICamera) Close() {
}
