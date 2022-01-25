//go:build !windows
// +build !windows

package externalcmd

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/kballard/go-shellquote"
)

func (e *Cmd) runInner() (int, bool) {
	cmdparts, err := shellquote.Split(e.cmdstr)
	if err != nil {
		return 0, true
	}

	cmd := exec.Command(cmdparts[0], cmdparts[1:]...)

	cmd.Env = append([]string(nil), os.Environ()...)
	for key, val := range e.env {
		cmd.Env = append(cmd.Env, key+"="+val)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Start()
	if err != nil {
		return 0, true
	}

	cmdDone := make(chan int)
	go func() {
		cmdDone <- func() int {
			err := cmd.Wait()
			if err == nil {
				return 0
			}
			ee, ok := err.(*exec.ExitError)
			if !ok {
				return 0
			}
			return ee.ExitCode()
		}()
	}()

	select {
	case <-e.terminate:
		syscall.Kill(cmd.Process.Pid, syscall.SIGINT)
		<-cmdDone
		return 0, false

	case c := <-cmdDone:
		return c, true
	}
}
