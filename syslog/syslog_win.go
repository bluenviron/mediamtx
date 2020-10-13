// +build windows

package syslog

import (
	"fmt"
)

type syslog struct {
}

func New(prefix string) (io.WriteCloser, error) {
	return nil, fmt.Errorf("not implemented on windows")
}

func (ls *syslog) Close() error {
	return nil
}

func (ls *syslog) Write(p []byte) (int, error) {
	return 0, nil
}
