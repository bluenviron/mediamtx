package logger

// Level is a log level.
type Level int

// Log levels.
const (
	Debug Level = iota + 1
	Info
	Warn
	Error
)
