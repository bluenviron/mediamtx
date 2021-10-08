package conf

import (
	"encoding/json"
	"fmt"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

// LogDestinations is the logDestionations parameter.
type LogDestinations map[logger.Destination]struct{}

// MarshalJSON marshals a LogDestinations into JSON.
func (d LogDestinations) MarshalJSON() ([]byte, error) {
	out := make([]string, len(d))
	i := 0

	for p := range d {
		var v string

		switch p {
		case logger.DestinationStdout:
			v = "stdout"

		case logger.DestinationFile:
			v = "file"

		default:
			v = "syslog"
		}

		out[i] = v
		i++
	}

	return json.Marshal(out)
}

// UnmarshalJSON unmarshals a LogDestinations from JSON.
func (d *LogDestinations) UnmarshalJSON(b []byte) error {
	slice, err := unmarshalStringSlice(b)
	if err != nil {
		return err
	}

	*d = make(LogDestinations)

	for _, proto := range slice {
		switch proto {
		case "stdout":
			(*d)[logger.DestinationStdout] = struct{}{}

		case "file":
			(*d)[logger.DestinationFile] = struct{}{}

		case "syslog":
			(*d)[logger.DestinationSyslog] = struct{}{}

		default:
			return fmt.Errorf("invalid log destination: %s", proto)
		}
	}

	return nil
}
