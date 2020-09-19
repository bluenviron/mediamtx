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
		// in Windows the shell is not used and command is started directly
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
	e.cmd.Process.Signal(os.Interrupt)
	e.cmd.Wait()
}
