// Package errordumper contains a counter that that periodically invokes a callback if the counter is not zero.
package errordumper

import (
	"sync"
	"time"
)

const (
	callbackPeriod = 1 * time.Second
)

// Dumper is a counter that periodically invokes a callback if errors were added.
type Dumper struct {
	OnReport func(v uint64, last error)

	mutex   sync.Mutex
	counter uint64
	last    error

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

// Add adds an error to the counter.
func (c *Dumper) Add(err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.counter++
	c.last = err
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
			counter := c.counter
			last := c.last
			c.mutex.Unlock()

			if counter != 0 {
				c.OnReport(counter, last)
			}
		}
	}
}
