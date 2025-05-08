package logger

import (
	"time"
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

type destination interface {
	log(time.Time, Level, string, ...interface{})
	close()
}
