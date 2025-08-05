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
	pool    *Pool
	cmdstr  string
	restart bool
	env     Environment
	onExit  func(error)

	// in
	terminate chan struct{}
}

// NewCmd allocates a Cmd.
func NewCmd(
	pool *Pool,
	cmdstr string,
	restart bool,
	env Environment,
	onExit OnExitFunc,
) *Cmd {
	// replace variables in both Linux and Windows, in order to allow using the
	// same commands on both of them.
	cmdstr = os.Expand(cmdstr, func(variable string) string {
		if value, ok := env[variable]; ok {
			return value
		}
		return os.Getenv(variable)
	})

	if onExit == nil {
		onExit = func(_ error) {}
	}

	e := &Cmd{
		pool:      pool,
		cmdstr:    cmdstr,
		restart:   restart,
		env:       env,
		onExit:    onExit,
		terminate: make(chan struct{}),
	}

	pool.wg.Add(1)

	go e.run()

	return e
}

// Close closes the command. It doesn't wait for the command to exit.
func (e *Cmd) Close() {
	close(e.terminate)
}

func (e *Cmd) run() {
	defer e.pool.wg.Done()

	env := append([]string(nil), os.Environ()...)
	for key, val := range e.env {
		env = append(env, key+"="+val)
	}

	for {
		err := e.runOSSpecific(env)
		if errors.Is(err, errTerminated) {
			return
		}

		if !e.restart {
			if err != nil {
				e.onExit(err)
			}
			return
		}

		if err != nil {
			e.onExit(err)
		} else {
			e.onExit(fmt.Errorf("command exited with code 0"))
		}

		select {
		case <-time.After(restartPause):
		case <-e.terminate:
			return
		}
	}
}
