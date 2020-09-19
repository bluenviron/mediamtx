package main

import (
	"log"
	"os"
)

type logDestination int

const (
	logDestinationStdout logDestination = iota
	logDestinationFile
	logDestinationSyslog
)

type logHandler struct {
	logDestinations map[logDestination]struct{}
	logFile         *os.File
	logSyslog       *logSyslog
}

func newLogHandler(logDestinations map[logDestination]struct{}, logFilePath string) (*logHandler, error) {
	lh := &logHandler{
		logDestinations: logDestinations,
	}

	if _, ok := logDestinations[logDestinationFile]; ok {
		var err error
		lh.logFile, err = os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			lh.close()
			return nil, err
		}
	}

	if _, ok := logDestinations[logDestinationSyslog]; ok {
		var err error
		lh.logSyslog, err = newLogSyslog()
		if err != nil {
			lh.close()
			return nil, err
		}
	}

	log.SetOutput(lh)

	return lh, nil
}

func (lh *logHandler) close() {
	if lh.logFile != nil {
		lh.logFile.Close()
	}

	if lh.logSyslog != nil {
		lh.logSyslog.close()
	}
}

func (lh *logHandler) Write(p []byte) (int, error) {
	if _, ok := lh.logDestinations[logDestinationStdout]; ok {
		print(string(p))
	}

	if _, ok := lh.logDestinations[logDestinationFile]; ok {
		lh.logFile.Write(p)
	}

	if _, ok := lh.logDestinations[logDestinationSyslog]; ok {
		lh.logSyslog.write(p)
	}

	return len(p), nil
}
