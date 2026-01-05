// Package counterdumper contains a counter that that periodically invokes a callback if the counter is not zero.
package counterdumper

import (
	"sync/atomic"
	"time"
)

const (
	callbackPeriod = 1 * time.Second
)

// Dumper is a counter that periodically invokes a callback if the counter is not zero.
type Dumper struct {
	OnReport func(v uint64)

	counter *uint64

	terminate chan struct{}
	done      chan struct{}
}

// Start starts the counter.
func (c *Dumper) Start() {
	c.counter = new(uint64)
	c.terminate = make(chan struct{})
	c.done = make(chan struct{})

	go c.run()
}

// Stop stops the counter.
func (c *Dumper) Stop() {
	close(c.terminate)
	<-c.done
}

// Increase increases the counter value by 1.
func (c *Dumper) Increase() {
	atomic.AddUint64(c.counter, 1)
}

// Add adds value to the counter.
func (c *Dumper) Add(v uint64) {
	atomic.AddUint64(c.counter, v)
}

func (c *Dumper) run() {
	defer close(c.done)

	t := time.NewTicker(callbackPeriod)
	defer t.Stop()

	for {
		select {
		case <-c.terminate:
			return

		case <-t.C:
			v := atomic.SwapUint64(c.counter, 0)
			if v != 0 {
				c.OnReport(v)
			}
		}
	}
}
