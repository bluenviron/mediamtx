package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/aler9/rtsp-simple-server/internal/syslog"
)

// Level is a log level.
type Level int

// Log levels.
const (
	Debug Level = iota
	Info
	Warn
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
	buffer       []byte

	file   *os.File
	syslog io.WriteCloser
}

// New allocates a log handler.
func New(level Level, destinations map[Destination]struct{}, filePath string) (*Logger, error) {
	lh := &Logger{
		level:        level,
		destinations: destinations,
	}

	if _, ok := destinations[DestinationFile]; ok {
		var err error
		lh.file, err = os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			lh.Close()
			return nil, err
		}
	}

	if _, ok := destinations[DestinationSyslog]; ok {
		var err error
		lh.syslog, err = syslog.New("rtsp-simple-server")
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
func itoa(buf *[]byte, i int, wid int) {
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
	*buf = append(*buf, b[bp:]...)
}

// Log writes a log entry.
func (lh *Logger) Log(level Level, format string, args ...interface{}) {
	if level < lh.level {
		return
	}

	lh.mutex.Lock()
	defer lh.mutex.Unlock()

	lh.buffer = lh.buffer[:0]

	// date
	now := time.Now()
	year, month, day := now.Date()
	itoa(&lh.buffer, year, 4)
	lh.buffer = append(lh.buffer, '/')
	itoa(&lh.buffer, int(month), 2)
	lh.buffer = append(lh.buffer, '/')
	itoa(&lh.buffer, day, 2)
	lh.buffer = append(lh.buffer, ' ')

	// time
	hour, min, sec := now.Clock()
	itoa(&lh.buffer, hour, 2)
	lh.buffer = append(lh.buffer, ':')
	itoa(&lh.buffer, min, 2)
	lh.buffer = append(lh.buffer, ':')
	itoa(&lh.buffer, sec, 2)
	lh.buffer = append(lh.buffer, ' ')

	// level
	switch level {
	case Debug:
		lh.buffer = append(lh.buffer, "[D] "...)

	case Info:
		lh.buffer = append(lh.buffer, "[I] "...)

	case Warn:
		lh.buffer = append(lh.buffer, "[W] "...)
	}

	// content
	lh.buffer = append(lh.buffer, fmt.Sprintf(format, args...)...)
	lh.buffer = append(lh.buffer, '\n')

	// output
	if _, ok := lh.destinations[DestinationStdout]; ok {
		print(string(lh.buffer))
	}
	if _, ok := lh.destinations[DestinationFile]; ok {
		lh.file.Write(lh.buffer)
	}
	if _, ok := lh.destinations[DestinationSyslog]; ok {
		lh.syslog.Write(lh.buffer)
	}
}
