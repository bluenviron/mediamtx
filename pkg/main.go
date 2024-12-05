package mediamtx

import (
	"github.com/bluenviron/mediamtx/internal/core"
)

func Main(args []string) bool {
	s, ok := core.New(args)
	if !ok {
		return false
	}
	s.Wait()
	return true
}
