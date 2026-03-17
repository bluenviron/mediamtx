// main executable.
package main

import (
	"os"

	_ "github.com/KimMachineGun/automemlimit"

	"github.com/bluenviron/mediamtx/internal/core"
)

func main() {
	s, ok := core.New(os.Args[1:])
	if !ok {
		os.Exit(1)
	}
	s.Wait()
}
