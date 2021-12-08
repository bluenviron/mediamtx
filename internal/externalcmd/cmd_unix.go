//go:build !windows
// +build !windows

package externalcmd

import (
	"os"
	"os/exec"
	"syscall"
)

func (e *Cmd) runInner() bool {
	cmd := exec.Command("/bin/sh", "-c", "exec "+e.cmdstr)

	cmd.Env = append([]string(nil), os.Environ()...)
	for key, val := range e.env {
		cmd.Env = append(cmd.Env, key+"="+val)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Start()
	if err != nil {
		return true
	}

	cmdDone := make(chan struct{})
	go func() {
		defer close(cmdDone)
		cmd.Wait()
	}()

	select {
	case <-e.terminate:
		syscall.Kill(cmd.Process.Pid, syscall.SIGINT)
		<-cmdDone
		return false

	case <-cmdDone:
		return true
	}
}
