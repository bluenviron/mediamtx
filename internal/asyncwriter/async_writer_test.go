package asyncwriter

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAsyncWriter(t *testing.T) {
	w := New(512, nil)

	w.Start()
	defer w.Stop()

	w.Push(func() error {
		return fmt.Errorf("testerror")
	})

	err := <-w.Error()
	require.EqualError(t, err, "testerror")
}
