//go:build rpicamera
// +build rpicamera

package rpicamera

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"
)

const (
	tempPathPrefix = "/dev/shm/rtspss-embeddedexe-"
)

func getKernelArch() (string, error) {
	cmd := exec.Command("uname", "-m")

	byts, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(byts[:len(byts)-1]), nil
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

type embeddedExe struct {
	cmd *exec.Cmd
}

func newEmbeddedExe(content []byte, env []string) (*embeddedExe, error) {
	err := checkArch()
	if err != nil {
		return nil, err
	}

	tempPath := tempPathPrefix + strconv.FormatInt(time.Now().UnixNano(), 10)

	err = os.WriteFile(tempPath, content, 0o755)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(tempPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	err = cmd.Start()
	os.Remove(tempPath)

	if err != nil {
		return nil, err
	}

	return &embeddedExe{
		cmd: cmd,
	}, nil
}

func (e *embeddedExe) close() {
	e.cmd.Process.Kill()
	e.cmd.Wait()
}
