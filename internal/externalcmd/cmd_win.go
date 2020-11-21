// +build windows

package externalcmd

import (
	"os"
	"os/exec"
	"strings"

	"github.com/kballard/go-shellquote"
)

func (e *Cmd) runInner() bool {
	// on Windows the shell is not used and command is started directly
	// variables are replaced manually in order to guarantee compatibility
	// with Linux commands
	tmp := strings.ReplaceAll(e.cmdstr, "$RTSP_PATH", e.env.Path)
	tmp = strings.ReplaceAll(tmp, "$RTSP_PORT", e.env.Port)
	parts, err := shellquote.Split(tmp)
	if err != nil {
		return true
	}

	cmd := exec.Command(parts[0], parts[1:]...)

	cmd.Env = append(os.Environ(),
		"RTSP_PATH="+e.env.Path,
		"RTSP_PORT="+e.env.Port,
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Start()
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
		cmd.Process.Kill()
		<-cmdDone
		return false

	case <-cmdDone:
		return true
	}
}
