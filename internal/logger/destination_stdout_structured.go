package logger

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"time"
)

type destinationStdoutStructured struct {
	buf bytes.Buffer
}

func newDestionationStdoutStructured() destination {
	return &destinationStdoutStructured{}
}

func (d *destinationStdoutStructured) log(t time.Time, level Level, format string, args ...any) {
	d.buf.Reset()
	d.buf.WriteString(`{"ts":"`)
	writeTime(&d.buf, t, false)
	d.buf.WriteString(`","level":"`)
	writeLevel(&d.buf, level, false)
	d.buf.WriteString(`","msg":"`)
	d.buf.WriteString(strconv.Quote(fmt.Sprintf(format, args...)))
	d.buf.WriteString(`"}`)
	d.buf.WriteByte('\n')
	os.Stdout.Write(d.buf.Bytes()) //nolint:errcheck
}

func (d *destinationStdoutStructured) close() {
}
