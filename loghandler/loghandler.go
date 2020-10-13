package loghandler

import (
	"io"
	"log"
	"os"

	"github.com/aler9/rtsp-simple-server/syslog"
)

type Destination int

const (
	DestinationStdout Destination = iota
	DestinationFile
	DestinationSyslog
)

type LogHandler struct {
	destinations map[Destination]struct{}
	file         *os.File
	syslog       io.WriteCloser
}

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

	log.SetOutput(lh)

	return lh, nil
}

func (lh *LogHandler) Close() {
	if lh.file != nil {
		lh.file.Close()
	}

	if lh.syslog != nil {
		lh.syslog.Close()
	}
}

func (lh *LogHandler) Write(p []byte) (int, error) {
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
