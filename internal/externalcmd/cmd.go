// Package externalcmd allows to launch external commands.
package externalcmd

import (
	"errors"
	"fmt"
	"os"
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
	OnExit  OnExitFunc

	// in
	terminate chan struct{}
}

// Start starts the command.
func (c *Cmd) Start() {
	// replace variables in both Linux and Windows, in order to allow using the
	// same commands on both of them.
	c.Cmdstr = os.Expand(c.Cmdstr, func(variable string) string {
		if value, ok := c.Env[variable]; ok {
			return value
		}
		return os.Getenv(variable)
	})

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

func (c *Cmd) run() {
	defer c.Pool.wg.Done()

	env := append([]string(nil), os.Environ()...)
	for key, val := range c.Env {
		env = append(env, key+"="+val)
	}

	for {
		err := c.runOSSpecific(env)
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
