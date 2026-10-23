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

func TestOnAvailableOutputLogger(t *testing.T) {
	pool := &externalcmd.Pool{}
	pool.Initialize()
	defer pool.Close()
	pool.LogOutput.Store(true)

	var mutex sync.Mutex
	var entries []string

	closeHook := OnAvailable(OnAvailableParams{
		Logger: test.Logger(func(_ logger.Level, format string, args ...any) {
			mutex.Lock()
			defer mutex.Unlock()
			entries = append(entries, fmt.Sprintf(format, args...))
		}),
		ExternalCmdPool: pool,
		Conf: &conf.Path{
			RunOnAvailable:   "sh -c 'echo runOnAvailable output'",
			RunOnUnavailable: "sh -c 'echo runOnUnavailable output'",
		},
		ExternalCmdEnv: externalcmd.Environment{},
	})

	require.Eventually(t, func() bool {
		mutex.Lock()
		defer mutex.Unlock()
		return slices.Contains(entries, "runOnAvailable output")
	}, 10*time.Second, 100*time.Millisecond)

	closeHook()

	require.Eventually(t, func() bool {
		mutex.Lock()
		defer mutex.Unlock()
		return slices.Contains(entries, "runOnUnavailable output")
	}, 10*time.Second, 100*time.Millisecond)
}
