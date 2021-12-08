//go:build windows
// +build windows

package externalcmd

import (
	"os"
	"os/exec"
	"strings"

	"github.com/kballard/go-shellquote"
)

func (e *Cmd) runInner() (int, bool) {
	// On Windows, the shell is not used and command is started directly.
	// Variables are replaced manually in order to guarantee compatibility
	// with Linux commands.
	tmp := e.cmdstr
	for key, val := range e.env {
		tmp = strings.ReplaceAll(tmp, "$"+key, val)
	}
	parts, err := shellquote.Split(tmp)
	if err != nil {
		return 0, true
	}

	cmd := exec.Command(parts[0], parts[1:]...)

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
		// on Windows, it's not possible to send os.Interrupt to a process.
		// Kill() is the only supported way.
		cmd.Process.Kill()
		<-cmdDone
		return 0, false

	case c := <-cmdDone:
		return c, true
	}
}
