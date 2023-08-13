package logger

import (
	"bytes"
	"os"
	"time"
)

type destinationFile struct {
	file *os.File
	buf  bytes.Buffer
}

func newDestinationFile(filePath string) (destination, error) {
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	return &destinationFile{
		file: f,
	}, nil
}

func (d *destinationFile) log(t time.Time, level Level, format string, args ...interface{}) {
	d.buf.Reset()
	writeTime(&d.buf, t, false)
	writeLevel(&d.buf, level, false)
	writeContent(&d.buf, format, args)
	d.file.Write(d.buf.Bytes()) //nolint:errcheck
}

func (d *destinationFile) close() {
	d.file.Close()
}
