// +build !windows

package syslog

import (
	"io"
	native "log/syslog"
)

type syslog struct {
	inner *native.Writer
}

// New allocates a io.WriteCloser that writes to the system log.
func New(prefix string) (io.WriteCloser, error) {
	inner, err := native.New(native.LOG_INFO|native.LOG_DAEMON, prefix)
	if err != nil {
		return nil, err
	}

	return &syslog{
		inner: inner,
	}, nil
}

// Close implements io.WriteCloser.
func (ls *syslog) Close() error {
	return ls.inner.Close()
}

// Write implements io.WriteCloser.
func (ls *syslog) Write(p []byte) (int, error) {
	return ls.inner.Write(p)
}
