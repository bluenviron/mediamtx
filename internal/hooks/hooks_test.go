package hooks_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/hooks"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/test"
)

func countWithPrefix(logged map[string]int, prefix string) int {
	n := 0
	for msg, count := range logged {
		if strings.HasPrefix(msg, prefix) {
			n += count
		}
	}
	return n
}

func TestHookCommandExited(t *testing.T) {
	cmdstr := filepath.Join(t.TempDir(), "nonexistent")
	pathConf := &conf.Path{
		RunOnOffline:     cmdstr,
		RunOnUnavailable: cmdstr,
		RunOnUnread:      cmdstr,
		RunOnUnDemand:    cmdstr,
	}

	var mutex sync.Mutex
	logged := map[string]int{}

	l := test.Logger(func(_ logger.Level, format string, args ...any) {
		mutex.Lock()
		defer mutex.Unlock()
		logged[fmt.Sprintf(format, args...)]++
	})

	p := &externalcmd.Pool{}
	p.Initialize()

	hooks.OnOnline(hooks.OnOnlineParams{
		Logger:          l,
		ExternalCmdPool: p,
		Conf:            pathConf,
		ExternalCmdEnv:  externalcmd.Environment{},
	})()
	hooks.OnAvailable(hooks.OnAvailableParams{
		Logger:          l,
		ExternalCmdPool: p,
		Conf:            pathConf,
		ExternalCmdEnv:  externalcmd.Environment{},
	})()
	hooks.OnRead(hooks.OnReadParams{
		Logger:          l,
		ExternalCmdPool: p,
		Conf:            pathConf,
		ExternalCmdEnv:  externalcmd.Environment{},
	})()
	hooks.OnDemand(hooks.OnDemandParams{
		Logger:          l,
		ExternalCmdPool: p,
		Conf:            pathConf,
		ExternalCmdEnv:  externalcmd.Environment{},
	})("test")
	hooks.OnConnect(hooks.OnConnectParams{
		Logger:          l,
		ExternalCmdPool: p,
		RunOnDisconnect: cmdstr,
	})()

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

	for _, hook := range []string{"runOnOffline", "runOnUnavailable", "runOnUnread", "runOnUnDemand", "runOnDisconnect"} {
		require.Equal(t, 1, countWithPrefix(logged, hook+" command exited with error: "), hook)
	}
}
