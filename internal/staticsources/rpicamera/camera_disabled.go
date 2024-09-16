//go:build !linux || (!arm && !arm64)
// +build !linux !arm,!arm64

package rpicamera

import (
	"fmt"
	"time"
)

type camera struct {
	Params params
	OnData func(time.Duration, [][]byte)
}

func (c *camera) initialize() error {
	return fmt.Errorf("server was compiled without support for the Raspberry Pi Camera")
}

func (c *camera) close() {
}

func (c *camera) reloadParams(_ params) {
}
