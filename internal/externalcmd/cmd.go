// Package externalcmd allows to launch external commands.
package externalcmd

import (
	"strings"
	"time"
)

const (
	restartPause = 5 * time.Second
)

// Environment is a Cmd environment.
type Environment map[string]string

// Cmd is an external command.
type Cmd struct {
	pool    *Pool
	cmdstr  string
	restart bool
	env     Environment
	onExit  func(int)

	// in
	terminate chan struct{}
}

// NewCmd allocates a Cmd.
func NewCmd(
	pool *Pool,
	cmdstr string,
	restart bool,
	env Environment,
	onExit func(int),
) *Cmd {
	// replace variables in both Linux and Windows, in order to allow using the
	// same commands on both of them.
	for key, val := range env {
		cmdstr = strings.ReplaceAll(cmdstr, "$"+key, val)
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

	for {
		ok := e.runInner()
		if !ok {
			break
		}
	}
}

func (e *Cmd) runInner() bool {
	c, ok := e.runOSSpecific()
	if !ok {
		return false
	}

	e.onExit(c)

	if !e.restart {
		<-e.terminate
		return false
	}

	select {
	case <-time.After(restartPause):
		return true
	case <-e.terminate:
		return false
	}
}
