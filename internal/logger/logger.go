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

	// for tests only
	TimeNow func() time.Time
	Stdout  io.Writer

	instance *instance
	mutex    sync.Mutex
}

// Initialize initializes Logger.
func (l *Logger) Initialize() error {
	if l.TimeNow == nil {
		l.TimeNow = time.Now
	}
	if l.Stdout == nil {
		l.Stdout = os.Stdout
	}

	return l.createInstance()
}

func (l *Logger) createInstance() error {
	i := &instance{
		Level:        l.Level,
		Destinations: l.Destinations,
		Structured:   l.Structured,
		File:         l.File,
		SysLogPrefix: l.SysLogPrefix,
		Stdout:       l.Stdout,
	}
	err := i.initialize()
	if err != nil {
		return err
	}

	l.instance = i
	return nil
}

// Reload applies the current values of the configuration fields.
func (l *Logger) Reload() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	old := l.instance
	l.closeInstance()

	err := l.createInstance()
	if err != nil {
		// restore the previous instance
		l.Level = old.Level
		l.Destinations = old.Destinations
		l.Structured = old.Structured
		l.File = old.File
		l.SysLogPrefix = old.SysLogPrefix
		l.createInstance() //nolint:errcheck
		return err
	}

	return nil
}

func (l *Logger) closeInstance() {
	l.instance.close()
	l.instance = nil
}

// Close closes a log handler.
func (l *Logger) Close() {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.closeInstance()
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
}

// Log writes a log entry.
func (l *Logger) Log(level Level, format string, args ...any) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.instance.log(l.TimeNow(), level, format, args...)
}
