//go:build linux && arm
// +build linux,arm

package rpicamera

import (
	"embed"
)

//go:embed mtxrpicam_32/*
var component embed.FS
