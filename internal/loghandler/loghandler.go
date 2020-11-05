package loghandler

import (
	"io"
	"log"
	"os"

	"github.com/aler9/rtsp-simple-server/internal/syslog"
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

type writeFunc func(p []byte) (int, error)

func (f writeFunc) Write(p []byte) (int, error) {
	return f(p)
}

// LogHandler is a log handler.
type LogHandler struct {
	destinations map[Destination]struct{}

	file   *os.File
	syslog io.WriteCloser
}

// New allocates a log handler.
func New(destinations map[Destination]struct{}, filePath string) (*LogHandler, error) {
	lh := &LogHandler{
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

	log.SetOutput(writeFunc(lh.write))

	return lh, nil
}

// Close closes a log handler.
func (lh *LogHandler) Close() {
	if lh.file != nil {
		lh.file.Close()
	}

	if lh.syslog != nil {
		lh.syslog.Close()
	}
}

func (lh *LogHandler) write(p []byte) (int, error) {
	if _, ok := lh.destinations[DestinationStdout]; ok {
		print(string(p))
	}

	if _, ok := lh.destinations[DestinationFile]; ok {
		lh.file.Write(p)
	}

	if _, ok := lh.destinations[DestinationSyslog]; ok {
		lh.syslog.Write(p)
	}

	return len(p), nil
}
