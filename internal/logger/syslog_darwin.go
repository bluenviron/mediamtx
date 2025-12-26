//go:build darwin

package logger

import (
	"fmt"
	"io"
)

func newSysLog(_ string) (io.WriteCloser, error) {
	return nil, fmt.Errorf("unavailable on macOS")
}
