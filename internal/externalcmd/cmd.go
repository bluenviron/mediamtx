// Package externalcmd allows to launch external commands.
package externalcmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

const (
	restartPause = 5 * time.Second
)

var errTerminated = errors.New("terminated")

// OnExitFunc is the prototype of onExit.
type OnExitFunc func(error)

// Environment is a Cmd environment.
type Environment map[string]string

// Cmd is an external command.
type Cmd struct {
	Pool    *Pool
	Cmdstr  string
	Restart bool
	Env     Environment
	Stdin   func() (io.ReadCloser, error)
	OnExit  OnExitFunc

	// in
	terminate chan struct{}
}

// Start starts the command.
func (c *Cmd) Start() {
	if c.OnExit == nil {
		c.OnExit = func(_ error) {}
	}

	c.terminate = make(chan struct{})

	c.Pool.wg.Add(1)

	go c.run()
}

// Close closes the command. It doesn't wait for the command to exit.
func (c *Cmd) Close() {
	close(c.terminate)
}

func expandEnv(s string, env Environment) string {
	return os.Expand(s, func(variable string) string {
		if value, ok := env[variable]; ok {
			return value
		}
		return os.Getenv(variable)
	})
}

func (c *Cmd) run() {
	defer c.Pool.wg.Done()

	env := append([]string(nil), os.Environ()...)
	for key, val := range c.Env {
		env = append(env, key+"="+val)
	}

	for {
		err := c.runOSSpecific(c.Cmdstr, env)
		if errors.Is(err, errTerminated) {
			return
		}

		if !c.Restart {
			if err != nil {
				c.OnExit(err)
			}
			return
		}

		if err != nil {
			c.OnExit(err)
		} else {
			c.OnExit(fmt.Errorf("command exited with code 0"))
		}

		select {
		case <-time.After(restartPause):
		case <-c.terminate:
			return
		}
	}
}

// setupStdin connects c.Stdin to the command through a pipe owned by us.
// Assigning a non-file reader to exec.Cmd.Stdin would make Wait() block
// until the reader yields data, even after the command has exited.
// The returned function must be called after Wait().
func (c *Cmd) setupStdin(cmd *exec.Cmd) (func(), error) {
	if c.Stdin == nil {
		return func() {}, nil
	}

	stdin, err := c.Stdin()
	if err != nil {
		return nil, err
	}

	w, err := cmd.StdinPipe()
	if err != nil {
		stdin.Close() //nolint:errcheck
		return nil, err
	}

	go func() {
		io.Copy(w, stdin) //nolint:errcheck
		w.Close()         //nolint:errcheck
	}()

	// closing stdin unblocks the copy goroutine.
	return func() { stdin.Close() }, nil //nolint:errcheck
}
