package logger

import (
	"io"
	"time"
)

type instance struct {
	Level        Level
	Destinations []Destination
	Structured   bool
	File         string
	SysLogPrefix string
	Stdout       io.Writer

	dests []destination
}

func (i *instance) initialize() error {
	for _, destType := range i.Destinations {
		switch destType {
		case DestinationStdout:
			i.dests = append(i.dests, newDestionationStdout(i.Structured, i.Stdout))

		case DestinationFile:
			dest, err := newDestinationFile(i.Structured, i.File)
			if err != nil {
				i.close()
				return err
			}
			i.dests = append(i.dests, dest)

		case DestinationSyslog:
			dest, err := newDestinationSyslog(i.SysLogPrefix)
			if err != nil {
				i.close()
				return err
			}
			i.dests = append(i.dests, dest)
		}
	}

	return nil
}

func (i *instance) close() {
	for _, dest := range i.dests {
		dest.close()
	}
	i.dests = nil
}

func (i *instance) log(t time.Time, level Level, format string, args ...any) {
	if level < i.Level {
		return
	}

	for _, dest := range i.dests {
		dest.log(t, level, format, args...)
	}
}
