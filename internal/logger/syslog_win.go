//go:build windows
// +build windows

package logger

import (
	"fmt"
	"io"
)

func newSyslog(prefix string) (io.WriteCloser, error) {
	return nil, fmt.Errorf("not implemented on windows")
}
