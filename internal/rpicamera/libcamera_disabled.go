//go:build !rpicamera
// +build !rpicamera

package rpicamera

// LibcameraSetup creates libcamera simlinks that are version agnostic.
func LibcameraSetup() {
}

// LibcameraCleanup removes files created by LibcameraSetup.
func LibcameraCleanup() {
}
