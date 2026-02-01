package logger

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"time"
)

type destinationFile struct {
	structured bool
	file       *os.File
	buf        bytes.Buffer
}

func newDestinationFile(structured bool, filePath string) (destination, error) {
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	return &destinationFile{
		structured: structured,
		file:       f,
	}, nil
}

func (d *destinationFile) log(t time.Time, level Level, format string, args ...any) {
	d.buf.Reset()

	if d.structured {
		d.buf.WriteString(`{"timestamp":"`)
		d.buf.WriteString(t.Format(time.RFC3339Nano))
		d.buf.WriteString(`","level":"`)
		writeLevel(&d.buf, level, false)
		d.buf.WriteString(`","message":`)
		d.buf.WriteString(strconv.Quote(fmt.Sprintf(format, args...)))
		d.buf.WriteString(`}`)
		d.buf.WriteByte('\n')
	} else {
		writePlainTime(&d.buf, t, false)
		writeLevel(&d.buf, level, false)
		d.buf.WriteByte(' ')
		fmt.Fprintf(&d.buf, format, args...)
		d.buf.WriteByte('\n')
	}

	d.file.Write(d.buf.Bytes()) //nolint:errcheck
}

func (d *destinationFile) close() {
	d.file.Close()
}
