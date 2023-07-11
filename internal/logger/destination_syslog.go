package logger

import (
	"bytes"
	"io"
	"time"
)

type destinationSysLog struct {
	syslog io.WriteCloser
	buf    bytes.Buffer
}

func newDestinationSyslog() (destination, error) {
	syslog, err := newSysLog("mediamtx")
	if err != nil {
		return nil, err
	}

	return &destinationSysLog{
		syslog: syslog,
	}, nil
}

func (d *destinationSysLog) log(t time.Time, level Level, format string, args ...interface{}) {
	d.buf.Reset()
	writeTime(&d.buf, t, false)
	writeLevel(&d.buf, level, false)
	writeContent(&d.buf, format, args)
	d.syslog.Write(d.buf.Bytes())
}

func (d *destinationSysLog) close() {
	d.syslog.Close()
}
