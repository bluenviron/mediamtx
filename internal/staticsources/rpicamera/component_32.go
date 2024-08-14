//go:build linux && arm
// +build linux,arm

package rpicamera

import (
	_ "embed"
)

//go:embed mtxrpicam_32
var component []byte
