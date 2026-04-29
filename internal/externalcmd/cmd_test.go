//go:build !windows

package externalcmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCmdRunExpandAfterSplit(t *testing.T) {
	// if os.Expand runs before shellquote.Split, a variable value containing a
	// single quote produces unbalanced quotes that cause shellquote.Split to fail.
	p := &Pool{}
	p.Initialize()

	out := filepath.Join(t.TempDir(), "out")

	cmd := &Cmd{
		Pool:   p,
		Cmdstr: "sh -c 'echo \"$MY_VAR\" > " + out + "'",
		Env: Environment{
			"MY_VAR": "it's",
		},
	}
	cmd.Start()

	poolClosed := make(chan struct{})
	go func() {
		p.Close()
		close(poolClosed)
	}()

	select {
	case <-poolClosed:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout")
	}

	byts, err := os.ReadFile(out)
	require.NoError(t, err)
	require.Equal(t, "it's\n", string(byts))
}
