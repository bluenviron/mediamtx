//go:build (linux && arm) || (linux && arm64)
// +build linux,arm linux,arm64

package rpicamera

import (
	"debug/elf"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	libraryToCheckArchitecture = "libc.so.6"
	dumpPrefix                 = "/dev/shm/mediamtx-rpicamera-"
	executableName             = "mtxrpicam"
)

var (
	dumpMutex sync.Mutex
	dumpCount = 0
	dumpPath  = ""
)

func getArchitecture(libPath string) (bool, error) {
	f, err := os.Open(libPath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	ef, err := elf.NewFile(f)
	if err != nil {
		return false, err
	}
	defer ef.Close()

	return (ef.FileHeader.Class == elf.ELFCLASS64), nil
}

func checkArchitecture() error {
	byts, err := exec.Command("ldconfig", "-p").Output()
	if err != nil {
		return fmt.Errorf("ldconfig failed: %w", err)
	}

	for _, line := range strings.Split(string(byts), "\n") {
		f := strings.Split(line, " => ")
		if len(f) == 2 && strings.Contains(f[1], libraryToCheckArchitecture) {
			is64, err := getArchitecture(f[1])
			if err != nil {
				return err
			}

			if runtime.GOARCH == "arm" {
				if !is64 {
					return nil
				}
			} else {
				if is64 {
					return nil
				}
			}
		}
	}

	if runtime.GOARCH == "arm" {
		return fmt.Errorf("the operating system is 64-bit, you need the 64-bit server version")
	}

	return fmt.Errorf("the operating system is 32-bit, you need the 32-bit server version")
}

func dumpEmbedFSRecursive(src string, dest string) error {
	files, err := component.ReadDir(src)
	if err != nil {
		return err
	}

	for _, f := range files {
		if f.IsDir() {
			err = os.Mkdir(filepath.Join(dest, f.Name()), 0o755)
			if err != nil {
				return err
			}

			err = dumpEmbedFSRecursive(filepath.Join(src, f.Name()), filepath.Join(dest, f.Name()))
			if err != nil {
				return err
			}
		} else {
			buf, err := component.ReadFile(filepath.Join(src, f.Name()))
			if err != nil {
				return err
			}

			err = os.WriteFile(filepath.Join(dest, f.Name()), buf, 0o644)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func dumpComponent() error {
	dumpMutex.Lock()
	defer dumpMutex.Unlock()

	if dumpCount > 0 {
		dumpCount++
		return nil
	}

	err := checkArchitecture()
	if err != nil {
		return err
	}

	dumpPath = dumpPrefix + strconv.FormatInt(time.Now().UnixNano(), 10)

	err = os.Mkdir(dumpPath, 0o755)
	if err != nil {
		return err
	}

	files, err := component.ReadDir(".")
	if err != nil {
		os.RemoveAll(dumpPath)
		return err
	}

	err = dumpEmbedFSRecursive(files[0].Name(), dumpPath)
	if err != nil {
		os.RemoveAll(dumpPath)
		return err
	}

	err = os.Chmod(filepath.Join(dumpPath, executableName), 0o755)
	if err != nil {
		os.RemoveAll(dumpPath)
		return err
	}

	dumpCount++

	return nil
}

func freeComponent() {
	dumpMutex.Lock()
	defer dumpMutex.Unlock()

	dumpCount--

	if dumpCount == 0 {
		os.RemoveAll(dumpPath)
	}
}
