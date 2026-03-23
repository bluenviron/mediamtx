// main executable.
package main

import (
	"log/slog"
	"os"

	"github.com/KimMachineGun/automemlimit/memlimit"
	"github.com/bluenviron/mediamtx/internal/core"
)

func init() {
	memlimit.SetGoMemLimitWithOpts(
		memlimit.WithRatio(0.7),
		memlimit.WithLogger(slog.Default()),
	)
}

func main() {
	s, ok := core.New(os.Args[1:])
	if !ok {
		os.Exit(1)
	}
	s.Wait()
}
