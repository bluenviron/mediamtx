package externalcmd_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestCmdRunExpandAfterSplit(t *testing.T) {
	// if os.Expand runs before shellquote.Split, a variable value containing a
	// single quote produces unbalanced quotes that cause shellquote.Split to fail.
	p := &externalcmd.Pool{}
	p.Initialize()

	out := filepath.Join(t.TempDir(), "out")

	cmd := &externalcmd.Cmd{
		Label:  "test",
		Parent: test.NilLogger,
		Pool:   p,
		Cmdstr: "sh -c 'echo \"$MY_VAR\" > " + out + "'",
		Env: externalcmd.Environment{
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
			p := &externalcmd.Pool{}
			p.Initialize()
			// Close() only waits on the WaitGroup: cmd.Close() must run first
			// (defers are LIFO), otherwise the restart case never returns.
			defer p.Close()

			exited := make(chan string, 1)

			cmd := &externalcmd.Cmd{
				Label: "test",
				Parent: test.Logger(func(_ logger.Level, format string, args ...any) {
					msg := fmt.Sprintf(format, args...)
					if strings.Contains(msg, "exited") {
						select {
						case exited <- msg:
						default:
						}
					}
				}),
				Pool:    p,
				Cmdstr:  "sh -c 'exit 5'",
				Restart: ca.restart,
			}
			cmd.Start()
			defer cmd.Close("")

			select {
			case msg := <-exited:
				require.Equal(t, "test command exited with error: returned code 5", msg)
			case <-time.After(10 * time.Second):
				t.Fatal("timeout")
			}
		})
	}
}

func TestCmdExitCodeZeroRestart(t *testing.T) {
	p := &externalcmd.Pool{}
	p.Initialize()
	defer p.Close()

	exited := make(chan string, 1)

	cmd := &externalcmd.Cmd{
		Label: "test",
		Parent: test.Logger(func(_ logger.Level, format string, args ...any) {
			msg := fmt.Sprintf(format, args...)
			if strings.Contains(msg, "exited") {
				select {
				case exited <- msg:
				default:
				}
			}
		}),
		Pool:    p,
		Cmdstr:  "sh -c 'exit 0'",
		Restart: true,
	}
	cmd.Start()
	defer cmd.Close("")

	select {
	case msg := <-exited:
		require.Equal(t, "test command exited without errors, restarting", msg)
	case <-time.After(10 * time.Second):
		t.Fatal("timeout")
	}
}
