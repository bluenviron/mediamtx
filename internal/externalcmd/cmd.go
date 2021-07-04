package externalcmd

import (
	"time"
)

const (
	restartPause = 5 * time.Second
)

// Environment is a Cmd environment.
type Environment struct {
	Path string
	Port string
}

// Cmd is an external command.
type Cmd struct {
	cmdstr  string
	restart bool
	env     Environment

	// in
	terminate chan struct{}

	// out
	done chan struct{}
}

// New allocates an Cmd.
func New(cmdstr string, restart bool, env Environment) *Cmd {
	e := &Cmd{
		cmdstr:    cmdstr,
		restart:   restart,
		env:       env,
		terminate: make(chan struct{}),
		done:      make(chan struct{}),
	}

	go e.run()

	return e
}

// Close closes an Cmd.
func (e *Cmd) Close() {
	close(e.terminate)
	<-e.done
}

func (e *Cmd) run() {
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

			select {
			case <-time.After(restartPause):
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
