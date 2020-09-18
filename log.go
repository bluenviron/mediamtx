package main

import (
	"log"
	"os"
)

type logDestination int

const (
	logDestinationStdout logDestination = iota
	logDestinationFile
)

type logHandler struct {
	logDestinationsParsed map[logDestination]struct{}
	logFile               *os.File
}

func newLogHandler(logDestinationsParsed map[logDestination]struct{}, logFilePath string) (*logHandler, error) {
	lh := &logHandler{
		logDestinationsParsed: logDestinationsParsed,
	}

	if _, ok := logDestinationsParsed[logDestinationFile]; ok {
		var err error
		lh.logFile, err = os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
	}

	log.SetOutput(lh)

	return lh, nil
}

func (lh *logHandler) close() error {
	if lh.logFile != nil {
		lh.logFile.Close()
	}

	return nil
}

func (lh *logHandler) Write(p []byte) (int, error) {
	if _, ok := lh.logDestinationsParsed[logDestinationStdout]; ok {
		print(string(p))
	}

	if _, ok := lh.logDestinationsParsed[logDestinationFile]; ok {
		lh.logFile.Write(p)
	}

	return len(p), nil
}
