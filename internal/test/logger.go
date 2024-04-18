package test

import "github.com/bluenviron/mediamtx/internal/logger"

type nilLogger struct{}

func (nilLogger) Log(_ logger.Level, _ string, _ ...interface{}) {
}

// NilLogger is a logger to /dev/null
var NilLogger logger.Writer = &nilLogger{}
