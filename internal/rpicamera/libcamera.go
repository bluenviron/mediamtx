//go:build rpicamera
// +build rpicamera

package rpicamera

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func findLibrary(name string) (string, error) {
	byts, err := exec.Command("ldconfig", "-p").Output()
	if err == nil {
		for _, line := range strings.Split(string(byts), "\n") {
			f := strings.Split(line, " => ")
			if len(f) == 2 && strings.Contains(f[1], name+".so") {
				return f[1], nil
			}
		}
	}

	return "", fmt.Errorf("library '%s' not found", name)
}

func setupSymlink(name string) error {
	lib, err := findLibrary(name)
	if err != nil {
		return err
	}

	os.Remove("/dev/shm/" + name + ".so.x.x.x")
	return os.Symlink(lib, "/dev/shm/"+name+".so.x.x.x")
}

// LibcameraSetup creates libcamera simlinks that are version agnostic.
func LibcameraSetup() error {
	err := setupSymlink("libcamera")
	if err != nil {
		return err
	}

	return setupSymlink("libcamera-base")
}

// LibcameraCleanup removes files created by LibcameraSetup.
func LibcameraCleanup() {
	os.Remove("/dev/shm/libcamera-base.so.x.x.x")
	os.Remove("/dev/shm/libcamera.so.x.x.x")
}
