//go:build !rpicamera
// +build !rpicamera

package rpicamera

import (
	"fmt"
)

// RPICamera is a RPI Camera reader.
type RPICamera struct{}

// New allocates a RPICamera.
func New(
	params Params,
	onData func([][]byte),
) (*RPICamera, error) {
	return nil, fmt.Errorf("server was compiled without support for the Raspberry Pi Camera")
}

// Close closes a RPICamera.
func (c *RPICamera) Close() {
}
