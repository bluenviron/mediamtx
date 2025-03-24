package counterdumper

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCounterDumperReport(t *testing.T) {
	done := make(chan struct{})

	c := &CounterDumper{
		OnReport: func(v uint64) {
			require.Equal(t, uint64(3), v)
			close(done)
		},
	}
	c.Start()
	defer c.Stop()

	c.Add(2)
	c.Increase()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Errorf("should not happen")
	}
}

func TestCounterDumperDoNotReport(t *testing.T) {
	c := &CounterDumper{
		OnReport: func(_ uint64) {
			t.Errorf("should not happen")
		},
	}
	c.Start()
	defer c.Stop()

	<-time.After(2 * time.Second)
}
