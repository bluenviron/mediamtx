package test

import "github.com/bluenviron/mediamtx/internal/logger"

type nilLogger struct{}

func (nilLogger) Log(_ logger.Level, _ string, _ ...interface{}) {
}

// NilLogger is a logger to /dev/null
var NilLogger logger.Writer = &nilLogger{}

// Logger is a test logger.
type Logger func(logger.Level, string, ...interface{})

// Log implements logger.Writer.
func (l Logger) Log(level logger.Level, format string, args ...interface{}) {
	l(level, format, args...)
}
