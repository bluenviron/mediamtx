//go:build !windows
// +build !windows

package externalcmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/kballard/go-shellquote"
)

func (e *Cmd) runOSSpecific() error {
	cmdParts, err := shellquote.Split(e.cmdstr)
	if err != nil {
		return err
	}

	cmd := exec.Command(cmdParts[0], cmdParts[1:]...)

	cmd.Env = append([]string(nil), os.Environ()...)
	for key, val := range e.env {
		cmd.Env = append(cmd.Env, key+"="+val)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Start()
	if err != nil {
		return err
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
		syscall.Kill(cmd.Process.Pid, syscall.SIGINT) //nolint:errcheck
		<-cmdDone
		return errTerminated

	case c := <-cmdDone:
		return fmt.Errorf("command returned code %d", c)
	}
}
