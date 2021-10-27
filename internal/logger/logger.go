package logger

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/gookit/color"
)

// Level is a log level.
type Level int

// Log levels.
const (
	Debug Level = iota + 1
	Info
	Warn
	Error
)

// Destination is a log destination.
type Destination int

const (
	// DestinationStdout writes logs to the standard output.
	DestinationStdout Destination = iota

	// DestinationFile writes logs to a file.
	DestinationFile

	// DestinationSyslog writes logs to the system logger.
	DestinationSyslog
)

// Logger is a log handler.
type Logger struct {
	level        Level
	destinations map[Destination]struct{}

	mutex        sync.Mutex
	file         *os.File
	syslog       io.WriteCloser
	stdoutBuffer bytes.Buffer
	fileBuffer   bytes.Buffer
	syslogBuffer bytes.Buffer
}

// New allocates a log handler.
func New(level Level, destinations map[Destination]struct{}, filePath string) (*Logger, error) {
	lh := &Logger{
		level:        level,
		destinations: destinations,
	}

	if _, ok := destinations[DestinationFile]; ok {
		var err error
		lh.file, err = os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			lh.Close()
			return nil, err
		}
	}

	if _, ok := destinations[DestinationSyslog]; ok {
		var err error
		lh.syslog, err = newSyslog("rtsp-simple-server")
		if err != nil {
			lh.Close()
			return nil, err
		}
	}

	return lh, nil
}

// Close closes a log handler.
func (lh *Logger) Close() {
	if lh.file != nil {
		lh.file.Close()
	}

	if lh.syslog != nil {
		lh.syslog.Close()
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

func writeTime(buf *bytes.Buffer, doColor bool) {
	var intbuf bytes.Buffer

	// date
	now := time.Now()
	year, month, day := now.Date()
	intbuf.Write(itoa(year, 4))
	intbuf.WriteByte('/')
	intbuf.Write(itoa(int(month), 2))
	intbuf.WriteByte('/')
	intbuf.Write(itoa(day, 2))
	intbuf.WriteByte(' ')

	// time
	hour, min, sec := now.Clock()
	intbuf.Write(itoa(hour, 2))
	intbuf.WriteByte(':')
	intbuf.Write(itoa(min, 2))
	intbuf.WriteByte(':')
	intbuf.Write(itoa(sec, 2))
	intbuf.WriteByte(' ')

	if doColor {
		buf.WriteString(color.RenderString(color.Gray.Code(), intbuf.String()))
	} else {
		buf.WriteString(intbuf.String())
	}
}

func writeLevel(buf *bytes.Buffer, level Level, doColor bool) {
	switch level {
	case Debug:
		if doColor {
			buf.WriteString(color.RenderString(color.Debug.Code(), "DEB"))
		} else {
			buf.WriteString("DEB")
		}

	case Info:
		if doColor {
			buf.WriteString(color.RenderString(color.Green.Code(), "INF"))
		} else {
			buf.WriteString("INF")
		}

	case Warn:
		if doColor {
			buf.WriteString(color.RenderString(color.Warn.Code(), "WAR"))
		} else {
			buf.WriteString("WAR")
		}

	case Error:
		if doColor {
			buf.WriteString(color.RenderString(color.Error.Code(), "ERR"))
		} else {
			buf.WriteString("ERR")
		}
	}
	buf.WriteByte(' ')
}

func writeContent(buf *bytes.Buffer, format string, args []interface{}) {
	buf.Write([]byte(fmt.Sprintf(format, args...)))
	buf.WriteByte('\n')
}

// Log writes a log entry.
func (lh *Logger) Log(level Level, format string, args ...interface{}) {
	if level < lh.level {
		return
	}

	lh.mutex.Lock()
	defer lh.mutex.Unlock()

	if _, ok := lh.destinations[DestinationStdout]; ok {
		lh.stdoutBuffer.Reset()
		writeTime(&lh.stdoutBuffer, true)
		writeLevel(&lh.stdoutBuffer, level, true)
		writeContent(&lh.stdoutBuffer, format, args)
		print(lh.stdoutBuffer.String())
	}

	if _, ok := lh.destinations[DestinationFile]; ok {
		lh.fileBuffer.Reset()
		writeTime(&lh.fileBuffer, false)
		writeLevel(&lh.fileBuffer, level, false)
		writeContent(&lh.fileBuffer, format, args)
		lh.file.Write(lh.fileBuffer.Bytes())
	}

	if _, ok := lh.destinations[DestinationSyslog]; ok {
		lh.syslogBuffer.Reset()
		writeTime(&lh.syslogBuffer, false)
		writeLevel(&lh.syslogBuffer, level, false)
		writeContent(&lh.syslogBuffer, format, args)
		lh.syslog.Write(lh.syslogBuffer.Bytes())
	}
}
