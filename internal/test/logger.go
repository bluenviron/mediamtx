package test

import "github.com/bluenviron/mediamtx/internal/logger"

type nilLogger struct{}

func (nilLogger) Log(_ logger.Level, _ string, _ ...any) {
}

// NilLogger is a logger to /dev/null
var NilLogger logger.Writer = &nilLogger{}

type testLogger struct {
	cb func(level logger.Level, format string, args ...any)
}

func (l *testLogger) Log(level logger.Level, format string, args ...any) {
	l.cb(level, format, args...)
}

// Logger returns a dummy logger.
func Logger(cb func(logger.Level, string, ...any)) logger.Writer {
	return &testLogger{cb: cb}
}
