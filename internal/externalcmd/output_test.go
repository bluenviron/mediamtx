package externalcmd_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestCmdOutput(t *testing.T) {
	for _, ca := range []struct {
		name    string
		cmd     string
		entries []string
	}{
		{"lines", `printf "a\nb\n"`, []string{"a", "b"}},
		{"crlf and empty lines", `printf "a\r\n\r\n\nb\r"`, []string{"a", "b\r"}},
		{"stderr and last line", `echo a; printf b >&2`, []string{"a", "b"}},
		{"long line", "printf " + strings.Repeat("a", 4096) + "b", []string{strings.Repeat("a", 4096), "b"}},
		{"percent", `printf "100%%s\n"`, []string{"100%s"}},
	} {
		t.Run(ca.name, func(t *testing.T) {
			p := &externalcmd.Pool{LogOutput: true}
			p.Initialize()

			var entries []string

			cmd := &externalcmd.Cmd{
				Pool:   p,
				Cmdstr: "sh -c '" + ca.cmd + "'",
				OutputLogger: test.Logger(func(_ logger.Level, format string, args ...any) {
					entries = append(entries, fmt.Sprintf(format, args...))
				}),
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

			require.Equal(t, ca.entries, entries)
		})
	}
}
