package logger

import (
	"bytes"
	"os"
	"time"

	"golang.org/x/term"
)

type destinationStdout struct {
	useColor bool

	buf bytes.Buffer
}

func newDestionationStdout() destination {
	return &destinationStdout{
		useColor: term.IsTerminal(int(os.Stdout.Fd())),
	}
}

func (d *destinationStdout) log(t time.Time, level Level, format string, args ...interface{}) {
	d.buf.Reset()
	writeTime(&d.buf, t, d.useColor)
	writeLevel(&d.buf, level, d.useColor)
	writeContent(&d.buf, format, args)
	os.Stdout.Write(d.buf.Bytes()) //nolint:errcheck
}

func (d *destinationStdout) close() {
}
