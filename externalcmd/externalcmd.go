package externalcmd

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	restartPause = 2 * time.Second
)

type ExternalCmd struct {
	cmdstr   string
	restart  bool
	pathName string

	// in
	terminate chan struct{}

	// out
	done chan struct{}
}

func New(cmdstr string, restart bool, pathName string) *ExternalCmd {
	e := &ExternalCmd{
		cmdstr:    cmdstr,
		restart:   restart,
		pathName:  pathName,
		terminate: make(chan struct{}),
		done:      make(chan struct{}),
	}

	go e.run()
	return e
}

func (e *ExternalCmd) Close() {
	close(e.terminate)
	<-e.done
}

func (e *ExternalCmd) run() {
	defer close(e.done)

	for {
		if !e.runInner() {
			break
		}
	}
}

func (e *ExternalCmd) runInner() bool {
	ok := e.runInnerInner()
	if !ok {
		return false
	}

	if !e.restart {
		<-e.terminate
		return false
	}

	t := time.NewTimer(restartPause)
	defer t.Stop()

	select {
	case <-t.C:
		return true
	case <-e.terminate:
		return false
	}
}

func (e *ExternalCmd) runInnerInner() bool {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// on Windows the shell is not used and command is started directly
		// variables are replaced manually in order to guarantee compatibility
		// with Linux commands
		args := strings.Fields(strings.ReplaceAll(e.cmdstr, "$RTSP_SERVER_PATH", e.pathName))
		cmd = exec.Command(args[0], args[1:]...)

	} else {
		cmd = exec.Command("/bin/sh", "-c", "exec "+e.cmdstr)
	}

	// variables are inserted into the environment
	cmd.Env = append(os.Environ(),
		"RTSP_SERVER_PATH="+e.pathName,
	)

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
		// on Windows it's not possible to send os.Interrupt to a process
		// Kill() is the only supported way
		if runtime.GOOS == "windows" {
			cmd.Process.Kill()
		} else {
			cmd.Process.Signal(os.Interrupt)
		}
		<-cmdDone
		return false

	case <-cmdDone:
		return true
	}
}
