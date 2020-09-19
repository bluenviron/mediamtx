// +build !windows

package main

import (
	"log/syslog"
)

type logSyslog struct {
	inner *syslog.Writer
}

func newLogSyslog() (*logSyslog, error) {
	inner, err := syslog.New(syslog.LOG_INFO|syslog.LOG_DAEMON, "rtsp-simple-server")
	if err != nil {
		return nil, err
	}

	return &logSyslog{
		inner: inner,
	}, nil
}

func (ls *logSyslog) close() {
	ls.inner.Close()
}

func (ls *logSyslog) write(p []byte) (int, error) {
	return ls.inner.Write(p)
}
