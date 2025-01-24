package test

import "github.com/bluenviron/mediamtx/internal/logger"

type nilLogger struct{}

func (nilLogger) Log(_ logger.Level, _ string, _ ...interface{}) {
}

// NilLogger is a logger to /dev/null
var NilLogger logger.Writer = &nilLogger{}

type testLogger struct {
	cb func(level logger.Level, format string, args ...interface{})
}

func (l *testLogger) Log(level logger.Level, format string, args ...interface{}) {
	l.cb(level, format, args...)
}

// Logger returns a dummy logger.
func Logger(cb func(logger.Level, string, ...interface{})) logger.Writer {
	return &testLogger{cb: cb}
}
