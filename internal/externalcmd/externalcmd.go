package externalcmd

import (
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
