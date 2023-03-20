//go:build rpicamera
// +build rpicamera

package rpicamera

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
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

// 32-bit embedded executables can't run on 64-bit.
func checkArch() error {
	if runtime.GOARCH != "arm" {
		return nil
	}

	arch, err := getKernelArch()
	if err != nil {
		return err
	}

	if arch == "aarch64" {
		return fmt.Errorf("OS is 64-bit, you need the arm64 server version")
	}

	return nil
}

// LibcameraSetup creates libcamera simlinks that are version agnostic.
func LibcameraSetup() error {
	err := checkArch()
	if err != nil {
		return err
	}

	err = setupSymlink("libcamera")
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
