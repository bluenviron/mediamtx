package logger

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"golang.org/x/term"
)

type destinationStdout struct {
	useColor bool
	buf      bytes.Buffer
}

func newDestionationStdout() destination {
	return &destinationStdout{
		useColor: term.IsTerminal(int(os.Stdout.Fd())),
	}
}

func (d *destinationStdout) log(t time.Time, level Level, format string, args ...any) {
	d.buf.Reset()
	writeTime(&d.buf, t, d.useColor)
	writeLevel(&d.buf, level, d.useColor)
	fmt.Fprintf(&d.buf, format, args...)
	d.buf.WriteByte('\n')
	os.Stdout.Write(d.buf.Bytes()) //nolint:errcheck
}

func (d *destinationStdout) close() {
}
