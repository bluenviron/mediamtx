//go:build windows

package logger

import (
	"fmt"
	"io"
)

func newSysLog(prefix string) (io.WriteCloser, error) {
	return nil, fmt.Errorf("not implemented on windows")
}
