// +build windows

package syslog

import (
	"fmt"
	"io"
)

// New allocates a io.WriteCloser that writes to the system log.
func New(prefix string) (io.WriteCloser, error) {
	return nil, fmt.Errorf("not implemented on windows")
}
