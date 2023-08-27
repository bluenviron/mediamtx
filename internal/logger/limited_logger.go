package logger

import (
	"sync"
	"time"
)

const (
	minIntervalBetweenWarnings = 1 * time.Second
)

type limitedLogger struct {
	w           Writer
	mutex       sync.Mutex
	lastPrinted time.Time
}

// NewLimitedLogger is a wrapper around a Writer that limits printed messages.
func NewLimitedLogger(w Writer) Writer {
	return &limitedLogger{
		w: w,
	}
}

// Log is the main logging function.
func (l *limitedLogger) Log(level Level, format string, args ...interface{}) {
	now := time.Now()
	l.mutex.Lock()
	if now.Sub(l.lastPrinted) >= minIntervalBetweenWarnings {
		l.lastPrinted = now
		l.w.Log(level, format, args...)
	}
	l.mutex.Unlock()
}
