//go:build linux && arm64

package rpicamera

import (
	"embed"
)

//go:embed mtxrpicam_64/*
var mtxrpicam embed.FS
