package externalcmd

import (
	"sync"
	"sync/atomic"
	"time"
)

// Pool is a pool of external commands.
type Pool struct {
	// LogOutput routes the output of commands to their OutputLogger, instead of
	// letting them inherit the standard streams of the server. It can change at
	// any time and is read when a command starts.
	LogOutput atomic.Bool

	// outputWaitDelay overrides the wait for the output of a command that exited;
	// zero selects the default. It is set by tests.
	outputWaitDelay time.Duration

	wg sync.WaitGroup
}

// Initialize initializes a Pool.
func (p *Pool) Initialize() {
}

// Close waits for all external commands to exit.
func (p *Pool) Close() {
	p.wg.Wait()
}
