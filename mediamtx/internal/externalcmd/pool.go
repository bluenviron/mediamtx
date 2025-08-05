package externalcmd

import (
	"sync"
)

// Pool is a pool of external commands.
type Pool struct {
	wg sync.WaitGroup
}

// Initialize initializes a Pool.
func (p *Pool) Initialize() {
}

// Close waits for all external commands to exit.
func (p *Pool) Close() {
	p.wg.Wait()
}
