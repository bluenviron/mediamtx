package externalcmd

import (
	"sync"
)

// Pool is a pool of external commands.
type Pool struct {
	wg sync.WaitGroup
}

// NewPool allocates a Pool.
func NewPool() *Pool {
	return &Pool{}
}

// Close waits for all external commands to exit.
func (p *Pool) Close() {
	p.wg.Wait()
}
