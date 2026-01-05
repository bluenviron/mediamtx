package errordumper

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDumperReport(t *testing.T) {
	done := make(chan struct{})

	c := &Dumper{
		OnReport: func(v uint64, last error) {
			require.Equal(t, uint64(1), v)
			require.EqualError(t, last, "test error")
			close(done)
		},
	}
	c.Start()
	defer c.Stop()

	c.Add(fmt.Errorf("test error"))

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Errorf("should not happen")
	}
}

func TestDumperDoNotReport(t *testing.T) {
	c := &Dumper{
		OnReport: func(_ uint64, _ error) {
			t.Errorf("should not happen")
		},
	}
	c.Start()
	defer c.Stop()

	<-time.After(2 * time.Second)
}
