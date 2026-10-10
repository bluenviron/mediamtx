package externalcmd

import (
	"bytes"
	"io"
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

func TestCmdExitCode(t *testing.T) {
	for _, ca := range []struct {
		name    string
		restart bool
	}{
		{"standard", false},
		{"restart", true},
	} {
		t.Run(ca.name, func(t *testing.T) {
			p := &Pool{}
			p.Initialize()
			// Close() only waits on the WaitGroup: cmd.Close() must run first
			// (defers are LIFO), otherwise the restart case never returns.
			defer p.Close()

			exited := make(chan error, 1)

			cmd := &Cmd{
				Pool:    p,
				Cmdstr:  "sh -c 'exit 5'",
				Restart: ca.restart,
				OnExit: func(err error) {
					select {
					case exited <- err:
					default:
					}
				},
			}
			cmd.Start()
			defer cmd.Close()

			select {
			case err := <-exited:
				require.EqualError(t, err, "command exited with code 5")
			case <-time.After(10 * time.Second):
				t.Fatal("timeout")
			}
		})
	}
}

func TestCmdStdin(t *testing.T) {
	p := &Pool{}
	p.Initialize()
	defer p.Close()

	out := filepath.Join(t.TempDir(), "out")

	cmd := &Cmd{
		Pool:   p,
		Cmdstr: "sh -c 'cat > " + out + "'",
		Stdin: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte("piped data\n"))), nil
		},
	}
	cmd.Start()
	defer cmd.Close()

	require.Eventually(t, func() bool {
		byts, err := os.ReadFile(out)
		return err == nil && string(byts) == "piped data\n"
	}, 5*time.Second, 100*time.Millisecond)
}

func TestCmdStdinIdleExit(t *testing.T) {
	// a Stdin that never yields data must not prevent Wait() from returning
	// once the command has exited.
	p := &Pool{}
	p.Initialize()
	defer p.Close()

	pr, pw := io.Pipe()
	defer pw.Close() //nolint:errcheck

	exited := make(chan struct{})

	cmd := &Cmd{
		Pool:   p,
		Cmdstr: "sh -c 'exit 3'",
		Stdin: func() (io.ReadCloser, error) {
			return pr, nil
		},
		OnExit: func(error) {
			close(exited)
		},
	}
	cmd.Start()
	defer cmd.Close()

	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("command exit was not detected")
	}
}
