package core

import (
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/logger"
)

type limitedLogger struct {
	w           logger.Writer
	mutex       sync.Mutex
	lastPrinted time.Time
}

func newLimitedLogger(w logger.Writer) *limitedLogger {
	return &limitedLogger{
		w: w,
	}
}

func (l *limitedLogger) Log(level logger.Level, format string, args ...interface{}) {
	now := time.Now()
	l.mutex.Lock()
	if now.Sub(l.lastPrinted) >= minIntervalBetweenWarnings {
		l.lastPrinted = now
		l.w.Log(level, format, args...)
	}
	l.mutex.Unlock()
}
