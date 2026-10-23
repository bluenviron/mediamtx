package hooks

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestOnConnectOutputLogger(t *testing.T) {
	pool := &externalcmd.Pool{}
	pool.Initialize()
	defer pool.Close()
	pool.LogOutput.Store(true)

	var mutex sync.Mutex
	var entries []string

	closeHook := OnConnect(OnConnectParams{
		Logger: test.Logger(func(_ logger.Level, format string, args ...any) {
			mutex.Lock()
			defer mutex.Unlock()
			entries = append(entries, fmt.Sprintf(format, args...))
		}),
		ExternalCmdPool: pool,
		RunOnConnect:    "sh -c 'echo runOnConnect output'",
		RunOnDisconnect: "sh -c 'echo runOnDisconnect output'",
	})

	require.Eventually(t, func() bool {
		mutex.Lock()
		defer mutex.Unlock()
		return slices.Contains(entries, "runOnConnect output")
	}, 10*time.Second, 100*time.Millisecond)

	closeHook()

	require.Eventually(t, func() bool {
		mutex.Lock()
		defer mutex.Unlock()
		return slices.Contains(entries, "runOnDisconnect output")
	}, 10*time.Second, 100*time.Millisecond)
}
