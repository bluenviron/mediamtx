//go:build rpicamera
// +build rpicamera

package rpicamera

import (
	"os"
	"os/exec"
	"strconv"
	"time"
)

const (
	tempPathPrefix = "/dev/shm/rtspss-embeddedexe-"
)

type embeddedExe struct {
	cmd *exec.Cmd
}

func newEmbeddedExe(content []byte, env []string) (*embeddedExe, error) {
	tempPath := tempPathPrefix + strconv.FormatInt(time.Now().UnixNano(), 10)

	err := os.WriteFile(tempPath, content, 0o755)
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
