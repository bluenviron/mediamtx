//go:build (linux && arm) || (linux && arm64)
// +build linux,arm linux,arm64

package rpicamera

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

//go:generate go run ./mtxrpicamdownloader

const (
	dumpPrefix = "/dev/shm/mediamtx-rpicamera-"
)

var (
	dumpMutex sync.Mutex
	dumpCount = 0
	dumpPath  = ""
)

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

	dumpPath = dumpPrefix + strconv.FormatInt(time.Now().UnixNano(), 10)

	err := os.Mkdir(dumpPath, 0o755)
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

	err = os.Chmod(filepath.Join(dumpPath, "exe"), 0o755)
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
