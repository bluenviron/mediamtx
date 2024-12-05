// main executable.
package main

import (
	"os"

	mediamtx "github.com/bluenviron/mediamtx/pkg"
)

func main() {
	ok := mediamtx.Main(os.Args[1:])
	if !ok {
		os.Exit(1)
	}
}
