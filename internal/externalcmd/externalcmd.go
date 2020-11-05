package externalcmd

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	retryPause = 5 * time.Second
)

// Environment is a ExternalCmd environment.
type Environment struct {
	Path string
	Port string
}

// ExternalCmd is an external command.
type ExternalCmd struct {
	cmdstr  string
	restart bool
	env     Environment

	// in
	terminate chan struct{}

	// out
	done chan struct{}
}

// New allocates an ExternalCmd.
func New(cmdstr string, restart bool, env Environment) *ExternalCmd {
	e := &ExternalCmd{
		cmdstr:    cmdstr,
		restart:   restart,
		env:       env,
		terminate: make(chan struct{}),
		done:      make(chan struct{}),
	}

	go e.run()
	return e
}

// Close closes an ExternalCmd.
func (e *ExternalCmd) Close() {
	close(e.terminate)
	<-e.done
}

func (e *ExternalCmd) run() {
	defer close(e.done)

	for {
		ok := func() bool {
			ok := e.runInner()
			if !ok {
				return false
			}

			if !e.restart {
				<-e.terminate
				return false
			}

			t := time.NewTimer(retryPause)
			defer t.Stop()

			select {
			case <-t.C:
				return true
			case <-e.terminate:
				return false
			}
		}()
		if !ok {
			break
		}
	}
}

func (e *ExternalCmd) runInner() bool {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// on Windows the shell is not used and command is started directly
		// variables are replaced manually in order to guarantee compatibility
		// with Linux commands
		tmp := strings.ReplaceAll(e.cmdstr, "$RTSP_PATH", e.env.Path)
		tmp = strings.ReplaceAll(tmp, "$RTSP_PORT", e.env.Port)

		args := strings.Fields(tmp)
		cmd = exec.Command(args[0], args[1:]...)

	} else {
		cmd = exec.Command("/bin/sh", "-c", "exec "+e.cmdstr)
	}

	cmd.Env = append(os.Environ(),
		"RTSP_PATH="+e.env.Path,
		"RTSP_PORT="+e.env.Port,
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
