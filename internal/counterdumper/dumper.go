// Package counterdumper contains a counter that that periodically invokes a callback if the counter is not zero.
package counterdumper

import (
	"sync"
	"time"
)

const (
	callbackPeriod = 1 * time.Second
)

// Dumper is a counter that periodically invokes a callback if the counter is not zero.
type Dumper struct {
	OnReport func(v uint64)

	mutex      sync.Mutex
	counter    uint64
	absCounter uint64

	terminate chan struct{}
	done      chan struct{}
}

// Start starts the counter.
func (c *Dumper) Start() {
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
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.counter++
	c.absCounter++
}

// Add adds value to the counter.
func (c *Dumper) Add(v uint64) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.counter += v
	c.absCounter += v
}

// Get returns the counter value.
func (c *Dumper) Get() uint64 {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.absCounter
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
			c.mutex.Lock()
			var v uint64
			v, c.counter = c.counter, 0
			c.mutex.Unlock()

			if v != 0 {
				c.OnReport(v)
			}
		}
	}
}
