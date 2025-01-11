//go:build windows

package externalcmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"github.com/kballard/go-shellquote"
	"golang.org/x/sys/windows"
)

// taken from
// https://gist.github.com/hallazzang/76f3970bfc949831808bbebc8ca15209
func createProcessGroup() (windows.Handle, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	_, err = windows.SetInformationJobObject(
		h,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)))
	if err != nil {
		return 0, err
	}

	return h, nil
}

func closeProcessGroup(h windows.Handle) error {
	return windows.CloseHandle(h)
}

func addProcessToGroup(h windows.Handle, p *os.Process) error {
	// Combine the required access rights
	access := uint32(windows.PROCESS_SET_QUOTA | windows.PROCESS_TERMINATE)

	processHandle, err := windows.OpenProcess(access, false, uint32(p.Pid))
	if err != nil {
		return fmt.Errorf("failed to open process: %v", err)
	}
	defer windows.CloseHandle(processHandle)

	err = windows.AssignProcessToJobObject(h, processHandle)
	if err != nil {
		return fmt.Errorf("failed to assign process to job object: %v", err)
	}

	return nil
}

func (e *Cmd) runOSSpecific(env []string) error {
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

	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// create a process group to kill all subprocesses
	g, err := createProcessGroup()
	if err != nil {
		return err
	}

	err = cmd.Start()
	if err != nil {
		return err
	}

	err = addProcessToGroup(g, cmd.Process)
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
		closeProcessGroup(g)
		<-cmdDone
		return errTerminated

	case c := <-cmdDone:
		closeProcessGroup(g)
		if c != 0 {
			return fmt.Errorf("command exited with code %d", c)
		}
		return nil
	}
}
