//go:build !windows

package externalcmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/kballard/go-shellquote"
)

func (e *Cmd) runOSSpecific(env []string) error {
	cmdParts, err := shellquote.Split(e.cmdstr)
	if err != nil {
		return err
	}

	cmd := exec.Command(cmdParts[0], cmdParts[1:]...)

	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// set process group in order to allow killing subprocesses
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

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
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				ee.ExitCode()
			}
			return 0
		}()
	}()

	select {
	case <-e.terminate:
		// the minus is needed to kill all subprocesses
		syscall.Kill(-cmd.Process.Pid, syscall.SIGINT) //nolint:errcheck
		<-cmdDone
		return errTerminated

	case c := <-cmdDone:
		if c != 0 {
			return fmt.Errorf("command exited with code %d", c)
		}
		return nil
	}
}
