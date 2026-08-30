//go:build !linux || (!arm && !arm64)

package rpicamera

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/defs"
)

// Run implements StaticSource.
func (s *Source) Run(_ defs.StaticSourceRunParams) error {
	return fmt.Errorf("server was compiled without support for the Raspberry Pi Camera")
}
