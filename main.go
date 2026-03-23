// main executable.
package main

import (
	"log/slog"
	"os"

	"github.com/KimMachineGun/automemlimit/memlimit"
	"github.com/bluenviron/mediamtx/internal/core"
)

func main() {
	if _, err := memlimit.SetGoMemLimitWithOpts(
		memlimit.WithRatio(0.7),
		memlimit.WithLogger(slog.Default()),
	); err != nil {
		slog.Default().Warn(
			"automemlimit: continuing without cgroup-derived GOMEMLIMIT",
			slog.Any("error", err),
		)
	}

	s, ok := core.New(os.Args[1:])
	if !ok {
		os.Exit(1)
	}
	s.Wait()
}
