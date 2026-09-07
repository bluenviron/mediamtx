package reorderer

import (
	"sync"

	"github.com/bluenviron/mediamtx/internal/logger"
)

const (
	maxPendingBytes = 100 * 1024 * 1024
)

// Orchestrator enforces a memory limit that is shared among multiple reorderers.
type Orchestrator struct {
	Parent logger.Writer

	maxPendingBytes int

	mu           sync.Mutex
	pendingBytes int
}

// Initialize initializes the orchestrator.
func (o *Orchestrator) Initialize() {
	if o.maxPendingBytes == 0 {
		o.maxPendingBytes = maxPendingBytes
	}
}

func (o *Orchestrator) addPendingBytes(n int) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.pendingBytes += n
	return o.pendingBytes <= o.maxPendingBytes
}

func (o *Orchestrator) removePendingBytes(n int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.pendingBytes -= n
}
