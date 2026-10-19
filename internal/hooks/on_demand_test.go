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

func TestOnDemandOutputLogger(t *testing.T) {
	pool := &externalcmd.Pool{}
	pool.Initialize()
	defer pool.Close()
	pool.LogOutput.Store(true)

	var mutex sync.Mutex
	var entries []string

	closeHook := OnDemand(OnDemandParams{
		Logger: test.Logger(func(_ logger.Level, format string, args ...any) {
			mutex.Lock()
			defer mutex.Unlock()
			entries = append(entries, fmt.Sprintf(format, args...))
		}),
		ExternalCmdPool: pool,
		Conf: &conf.Path{
			RunOnDemand:   "sh -c 'echo runOnDemand output'",
			RunOnUnDemand: "sh -c 'echo runOnUnDemand output'",
		},
		ExternalCmdEnv: externalcmd.Environment{},
	})

	require.Eventually(t, func() bool {
		mutex.Lock()
		defer mutex.Unlock()
		return slices.Contains(entries, "runOnDemand output")
	}, 10*time.Second, 100*time.Millisecond)

	closeHook("test")

	require.Eventually(t, func() bool {
		mutex.Lock()
		defer mutex.Unlock()
		return slices.Contains(entries, "runOnUnDemand output")
	}, 10*time.Second, 100*time.Millisecond)
}
