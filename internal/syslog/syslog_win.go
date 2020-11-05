// +build windows

package syslog

import (
	"fmt"
	"io"
)

type syslog struct {
}

// New allocates a io.WriteCloser that writes to the system log.
func New(prefix string) (io.WriteCloser, error) {
	return nil, fmt.Errorf("not implemented on windows")
}

// Close implements io.WriteCloser.
func (ls *syslog) Close() error {
	return nil
}

// Write implements io.WriteCloser.
func (ls *syslog) Write(p []byte) (int, error) {
	return 0, nil
}
