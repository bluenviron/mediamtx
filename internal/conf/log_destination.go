package conf

import (
	"encoding/json"
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// LogDestination represents a log destination.
type LogDestination logger.Destination

// MarshalJSON implements json.Marshaler.
func (d LogDestination) MarshalJSON() ([]byte, error) {
	switch d {
	case LogDestination(logger.DestinationStdout):
		return json.Marshal("stdout")

	case LogDestination(logger.DestinationFile):
		return json.Marshal("file")

	default:
		return json.Marshal("syslog")
	}
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *LogDestination) UnmarshalJSON(b []byte) error {
	var in string
	if err := jsonwrapper.Unmarshal(b, &in); err != nil {
		return err
	}

	switch in {
	case "stdout":
		*d = LogDestination(logger.DestinationStdout)

	case "file":
		*d = LogDestination(logger.DestinationFile)

	case "syslog":
		*d = LogDestination(logger.DestinationSyslog)

	default:
		return fmt.Errorf("invalid log destination: %s", in)
	}

	return nil
}
