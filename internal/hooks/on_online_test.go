//nolint:dupl
package hooks

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestOnOnlineOutputLogger(t *testing.T) {
	pool := &externalcmd.Pool{}
	pool.Initialize()
	defer pool.Close()
	pool.LogOutput.Store(true)

	var mutex sync.Mutex
	var entries []string

	closeHook := OnOnline(OnOnlineParams{
		Logger: test.Logger(func(_ logger.Level, format string, args ...any) {
			mutex.Lock()
			defer mutex.Unlock()
			entries = append(entries, fmt.Sprintf(format, args...))
		}),
		ExternalCmdPool: pool,
		Conf: &conf.Path{
			RunOnOnline:  "sh -c 'echo runOnOnline output'",
			RunOnOffline: "sh -c 'echo runOnOffline output'",
		},
		ExternalCmdEnv: externalcmd.Environment{},
	})

	require.Eventually(t, func() bool {
		mutex.Lock()
		defer mutex.Unlock()
		return slices.Contains(entries, "runOnOnline output")
	}, 10*time.Second, 100*time.Millisecond)

	closeHook()

	require.Eventually(t, func() bool {
		mutex.Lock()
		defer mutex.Unlock()
		return slices.Contains(entries, "runOnOffline output")
	}, 10*time.Second, 100*time.Millisecond)
}
