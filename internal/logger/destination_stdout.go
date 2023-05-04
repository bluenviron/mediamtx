package logger

import (
	"bytes"
	"os"
)

type destinationStdout struct {
	buf bytes.Buffer
}

func newDestionationStdout() destination {
	return &destinationStdout{}
}

func (d *destinationStdout) log(level Level, format string, args ...interface{}) {
	d.buf.Reset()
	writeTime(&d.buf, true)
	writeLevel(&d.buf, level, true)
	writeContent(&d.buf, format, args)
	os.Stdout.Write(d.buf.Bytes())
}

func (d *destinationStdout) close() {
}
