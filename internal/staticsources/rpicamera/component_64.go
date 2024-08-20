//go:build linux && arm64
// +build linux,arm64

package rpicamera

import (
	"embed"
)

//go:embed mtxrpicam_64/*
var component embed.FS
