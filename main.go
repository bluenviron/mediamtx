// main executable.
package main

import (
	"fmt"
	"os"

	"github.com/bluenviron/mediamtx/internal/core"
)

func main() {
	fmt.Println(1)
	s, ok := core.New(os.Args[1:])
	if !ok {
		os.Exit(1)
	}
	s.Wait()
}
