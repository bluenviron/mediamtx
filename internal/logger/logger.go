// Package logger contains a logger implementation.
package logger

import (
	"bytes"
	"io"
	"os"
	"sync"
	"time"

	"github.com/gookit/color"
)

// Logger is a log handler.
type Logger struct {
	Level        Level
	Destinations []Destination
	Structured   bool
	File         string
	SysLogPrefix string

	timeNow      func() time.Time
	stdout       io.Writer
	destinations []destination
	mutex        sync.Mutex
}

// Initialize initializes Logger.
func (l *Logger) Initialize() error {
	if l.timeNow == nil {
		l.timeNow = time.Now
	}
	if l.stdout == nil {
		l.stdout = os.Stdout
	}

	for _, destType := range l.Destinations {
		switch destType {
		case DestinationStdout:
			l.destinations = append(l.destinations, newDestionationStdout(l.Structured, l.stdout))

		case DestinationFile:
			dest, err := newDestinationFile(l.Structured, l.File)
			if err != nil {
				l.Close()
				return err
			}
			l.destinations = append(l.destinations, dest)

		case DestinationSyslog:
			dest, err := newDestinationSyslog(l.SysLogPrefix)
			if err != nil {
				l.Close()
				return err
			}
			l.destinations = append(l.destinations, dest)
		}
	}

	return nil
}

// Close closes a log handler.
func (l *Logger) Close() {
	for _, dest := range l.destinations {
		dest.close()
	}
}

// https://golang.org/src/log/log.go#L78
func itoa(i int, wid int) []byte {
	// Assemble decimal in reverse order.
	var b [20]byte
	bp := len(b) - 1
	for i >= 10 || wid > 1 {
		wid--
		q := i / 10
		b[bp] = byte('0' + i - q*10)
		bp--
		i = q
	}
	// i < 10
	b[bp] = byte('0' + i)
	return b[bp:]
}

func writePlainTime(buf *bytes.Buffer, t time.Time, useColor bool) {
	var intbuf bytes.Buffer

	// date
	year, month, day := t.Date()
	intbuf.Write(itoa(year, 4))
	intbuf.WriteByte('/')
	intbuf.Write(itoa(int(month), 2))
	intbuf.WriteByte('/')
	intbuf.Write(itoa(day, 2))
	intbuf.WriteByte(' ')

	// time
	hour, minute, sec := t.Clock()
	intbuf.Write(itoa(hour, 2))
	intbuf.WriteByte(':')
	intbuf.Write(itoa(minute, 2))
	intbuf.WriteByte(':')
	intbuf.Write(itoa(sec, 2))
	intbuf.WriteByte(' ')

	if useColor {
		buf.WriteString(color.RenderString(color.Gray.Code(), intbuf.String()))
	} else {
		buf.WriteString(intbuf.String())
	}
}

func writeLevel(buf *bytes.Buffer, level Level, useColor bool) {
	switch level {
	case Debug:
		if useColor {
			buf.WriteString(color.RenderString(color.Debug.Code(), "DEB"))
		} else {
			buf.WriteString("DEB")
		}

	case Info:
		if useColor {
			buf.WriteString(color.RenderString(color.Green.Code(), "INF"))
		} else {
			buf.WriteString("INF")
		}

	case Warn:
		if useColor {
			buf.WriteString(color.RenderString(color.Warn.Code(), "WAR"))
		} else {
			buf.WriteString("WAR")
		}

	case Error:
		if useColor {
			buf.WriteString(color.RenderString(color.Error.Code(), "ERR"))
		} else {
			buf.WriteString("ERR")
		}
	}
	buf.WriteByte(' ')
}

// Log writes a log entry.
func (l *Logger) Log(level Level, format string, args ...any) {
	if level < l.Level {
		return
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	t := l.timeNow()

	for _, dest := range l.destinations {
		dest.log(t, level, format, args...)
	}
}
