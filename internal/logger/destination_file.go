package logger

import (
	"bytes"
	"os"
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

func (d *destinationFile) log(level Level, format string, args ...interface{}) {
	d.buf.Reset()
	writeTime(&d.buf, false)
	writeLevel(&d.buf, level, false)
	writeContent(&d.buf, format, args)
	d.file.Write(d.buf.Bytes())
}

func (d *destinationFile) close() {
	d.file.Close()
}
