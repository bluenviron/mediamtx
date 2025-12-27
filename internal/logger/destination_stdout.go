package logger

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"golang.org/x/term"
)

type destinationStdout struct {
	structured bool
	stdout     io.Writer
	useColor   bool
	buf        bytes.Buffer
}

func newDestionationStdout(structured bool, stdout io.Writer) destination {
	return &destinationStdout{
		structured: structured,
		stdout:     stdout,
		useColor:   term.IsTerminal(int(os.Stdout.Fd())),
	}
}

func (d *destinationStdout) log(t time.Time, level Level, format string, args ...any) {
	d.buf.Reset()

	if d.structured {
		d.buf.WriteString(`{"timestamp":"`)
		writeTime(&d.buf, t, false)
		d.buf.WriteString(`","level":"`)
		writeLevel(&d.buf, level, false)
		d.buf.WriteString(`","message":"`)
		d.buf.WriteString(strconv.Quote(fmt.Sprintf(format, args...)))
		d.buf.WriteString(`"}`)
		d.buf.WriteByte('\n')
	} else {
		writeTime(&d.buf, t, d.useColor)
		writeLevel(&d.buf, level, d.useColor)
		fmt.Fprintf(&d.buf, format, args...)
		d.buf.WriteByte('\n')
	}

	d.stdout.Write(d.buf.Bytes()) //nolint:errcheck
}

func (d *destinationStdout) close() {
}
