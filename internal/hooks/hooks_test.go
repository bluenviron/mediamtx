package hooks

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestHookCommandExited(t *testing.T) {
	for _, hook := range []struct {
		name  string
		setup func(l logger.Writer, p *externalcmd.Pool, cmdstr string) func()
	}{
		{
			"runOnOffline",
			func(l logger.Writer, p *externalcmd.Pool, cmdstr string) func() {
				return OnOnline(OnOnlineParams{
					Logger:          l,
					ExternalCmdPool: p,
					Conf:            &conf.Path{RunOnOffline: cmdstr},
					ExternalCmdEnv:  externalcmd.Environment{},
				})
			},
		},
		{
			"runOnUnDemand",
			func(l logger.Writer, p *externalcmd.Pool, cmdstr string) func() {
				f := OnDemand(OnDemandParams{
					Logger:          l,
					ExternalCmdPool: p,
					Conf:            &conf.Path{RunOnUnDemand: cmdstr},
					ExternalCmdEnv:  externalcmd.Environment{},
				})
				return func() { f("test") }
			},
		},
		{
			"runOnUnavailable",
			func(l logger.Writer, p *externalcmd.Pool, cmdstr string) func() {
				return OnAvailable(OnAvailableParams{
					Logger:          l,
					ExternalCmdPool: p,
					Conf:            &conf.Path{RunOnUnavailable: cmdstr},
					ExternalCmdEnv:  externalcmd.Environment{},
				})
			},
		},
		{
			"runOnUnread",
			func(l logger.Writer, p *externalcmd.Pool, cmdstr string) func() {
				return OnRead(OnReadParams{
					Logger:          l,
					ExternalCmdPool: p,
					Conf:            &conf.Path{RunOnUnread: cmdstr},
					ExternalCmdEnv:  externalcmd.Environment{},
				})
			},
		},
		{
			"runOnDisconnect",
			func(l logger.Writer, p *externalcmd.Pool, cmdstr string) func() {
				return OnConnect(OnConnectParams{
					Logger:          l,
					ExternalCmdPool: p,
					RunOnDisconnect: cmdstr,
					RTSPAddress:     ":8554",
				})
			},
		},
	} {
		for _, ca := range []struct {
			name     string
			cmdstr   string
			expected int
		}{
			{"failure", filepath.Join(t.TempDir(), "nonexistent"), 1},
			{"exit1", "sh -c 'exit 1'", 1},
			{"success", "sh -c 'exit 0'", 0},
		} {
			t.Run(hook.name+"_"+ca.name, func(t *testing.T) {
				p := &externalcmd.Pool{}
				p.Initialize()

				var mutex sync.Mutex
				count := 0

				l := test.Logger(func(level logger.Level, format string, args ...any) {
					if level == logger.Info && format == hook.name+" command exited: %v" &&
						len(args) == 1 {
						// the argument must be the error itself, not another value.
						if _, ok := args[0].(error); !ok {
							return
						}

						mutex.Lock()
						defer mutex.Unlock()
						count++
					}
				})

				hook.setup(l, p, ca.cmdstr)()

				// Close() returns after every command goroutine called OnExit:
				// Start() adds to the WaitGroup synchronously and run() calls
				// OnExit before its deferred Done(). It only waits on the
				// WaitGroup, so a command that restarts would block it forever.
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

				mutex.Lock()
				defer mutex.Unlock()
				require.Equal(t, ca.expected, count)
			})
		}
	}
}
