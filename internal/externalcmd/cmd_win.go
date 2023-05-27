//go:build windows
// +build windows

package externalcmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/kballard/go-shellquote"
)

func (e *Cmd) runOSSpecific() error {
	var cmd *exec.Cmd

	// from Golang documentation:
	// On Windows, processes receive the whole command line as a single string and do their own parsing.
	// Command combines and quotes Args into a command line string with an algorithm compatible with
	// applications using CommandLineToArgvW (which is the most common way). Notable exceptions are
	// msiexec.exe and cmd.exe (and thus, all batch files), which have a different unquoting algorithm.
	// In these or other similar cases, you can do the quoting yourself and provide the full command
	// line in SysProcAttr.CmdLine, leaving Args empty.
	if strings.HasPrefix(e.cmdstr, "cmd ") || strings.HasPrefix(e.cmdstr, "cmd.exe ") {
		args := strings.TrimPrefix(strings.TrimPrefix(e.cmdstr, "cmd "), "cmd.exe ")

		cmd = exec.Command("cmd.exe")
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CmdLine: args,
		}
	} else {
		cmdParts, err := shellquote.Split(e.cmdstr)
		if err != nil {
			return err
		}

		cmd = exec.Command(cmdParts[0], cmdParts[1:]...)
	}

	cmd.Env = append([]string(nil), os.Environ()...)
	for key, val := range e.env {
		cmd.Env = append(cmd.Env, key+"="+val)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Start()
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
		// on Windows, it's not possible to send os.Interrupt to a process.
		// Kill() is the only supported way.
		cmd.Process.Kill()
		<-cmdDone
		return errTerminated

	case c := <-cmdDone:
		return fmt.Errorf("command returned code %d", c)
	}
}
