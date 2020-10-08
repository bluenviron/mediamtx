package main

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type externalCmd struct {
	cmd *exec.Cmd
}

func startExternalCommand(cmdstr string, pathName string) (*externalCmd, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// on Windows the shell is not used and command is started directly
		// variables are replaced manually in order to allow
		// compatibility with linux commands
		cmdstr = strings.ReplaceAll(cmdstr, "$RTSP_SERVER_PATH", pathName)
		args := strings.Fields(cmdstr)
		cmd = exec.Command(args[0], args[1:]...)

	} else {
		cmd = exec.Command("/bin/sh", "-c", "exec "+cmdstr)
	}

	// variables are available through environment variables
	cmd.Env = append(os.Environ(),
		"RTSP_SERVER_PATH="+pathName,
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Start()
	if err != nil {
		return nil, err
	}

	return &externalCmd{
		cmd: cmd,
	}, nil
}

func (e *externalCmd) close() {
	// on Windows it's not possible to send os.Interrupt to a process
	// Kill() is the only supported way
	if runtime.GOOS == "windows" {
		e.cmd.Process.Kill()
	} else {
		e.cmd.Process.Signal(os.Interrupt)
	}
	e.cmd.Wait()
}
