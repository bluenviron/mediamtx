//go:build linux && arm

package rpicamera

import (
	"embed"
)

//go:embed mtxrpicam_32/*
var mtxrpicam embed.FS
