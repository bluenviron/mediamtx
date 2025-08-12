//go:build !linux || (!arm && !arm64)

package rpicamera

import (
	"fmt"
	"time"
)

type camera struct {
	params          params
	onData          func(int64, time.Time, [][]byte)
	onDataSecondary func(int64, time.Time, []byte)
}

func (c *camera) initialize() error {
	return fmt.Errorf("server was compiled without support for the Raspberry Pi Camera")
}

func (c *camera) close() {
}

func (c *camera) reloadParams(_ params) {
}

func (c *camera) wait() error {
	return nil
}
