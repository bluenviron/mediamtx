//go:build !rpicamera
// +build !rpicamera

package rpicamera

// LibcameraSetup creates libcamera simlinks that are version agnostic.
func LibcameraSetup() error {
	return nil
}

// LibcameraCleanup removes files created by LibcameraSetup.
func LibcameraCleanup() {
}
