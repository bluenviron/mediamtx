// +build windows

package main

import (
	"fmt"
)

type logSyslog struct {
}

func newLogSyslog() (*logSyslog, error) {
	return nil, fmt.Errorf("not implemented on windows")
}

func (ls *logSyslog) close() {
}

func (ls *logSyslog) write(p []byte) (int, error) {
	return 0, nil
}
