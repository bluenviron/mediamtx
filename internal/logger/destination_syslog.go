//go:build !darwin && !windows

package logger

import (
	"bytes"
	"fmt"
	"log/syslog"
	"time"
)

type destinationSysLog struct {
	syslog *syslog.Writer
	buf    bytes.Buffer
}

func newDestinationSyslog(prefix string) (destination, error) {
	syslog, err := syslog.New(syslog.LOG_DAEMON, prefix)
	if err != nil {
		return nil, err
	}

	return &destinationSysLog{
		syslog: syslog,
	}, nil
}

func (d *destinationSysLog) log(_ time.Time, level Level, format string, args ...any) {
	d.buf.Reset()

	fmt.Fprintf(&d.buf, format, args...)

	switch level {
	case Debug:
		d.syslog.Debug(d.buf.String()) //nolint:errcheck
	case Info:
		d.syslog.Info(d.buf.String()) //nolint:errcheck
	case Warn:
		d.syslog.Warning(d.buf.String()) //nolint:errcheck
	case Error:
		d.syslog.Err(d.buf.String()) //nolint:errcheck
	}
}

func (d *destinationSysLog) close() {
	d.syslog.Close() //nolint:errcheck
}
