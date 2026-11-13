package externalcmd

import (
	"sync"
)

// Pool is a pool of external commands.
type Pool struct {
	LogOutput bool

	mutex sync.RWMutex
	wg    sync.WaitGroup
}

// Initialize initializes a Pool.
func (p *Pool) Initialize() {
}

// Close waits for all external commands to exit.
func (p *Pool) Close() {
	p.wg.Wait()
}

// ReloadLogOutput is called by core.Core.
func (p *Pool) ReloadLogOutput(logOutput bool) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.LogOutput = logOutput
}
