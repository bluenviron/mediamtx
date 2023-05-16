package conf

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// LogDestinations is the logDestionations parameter.
type LogDestinations []logger.Destination

// MarshalJSON implements json.Marshaler.
func (d LogDestinations) MarshalJSON() ([]byte, error) {
	out := make([]string, len(d))
	i := 0

	for _, p := range d {
		var v string

		switch p {
		case logger.DestinationStdout:
			v = "stdout"

		case logger.DestinationFile:
			v = "file"

		case logger.DestinationSyslog:
			v = "syslog"

		default:
			return nil, fmt.Errorf("invalid log destination: %v", p)
		}

		out[i] = v
		i++
	}

	return json.Marshal(out)
}

func (d *LogDestinations) contains(v logger.Destination) bool {
	for _, item := range *d {
		if item == v {
			return true
		}
	}
	return false
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *LogDestinations) UnmarshalJSON(b []byte) error {
	var in []string
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}

	*d = nil

	for _, dest := range in {
		var v logger.Destination
		switch dest {
		case "stdout":
			v = logger.DestinationStdout

		case "file":
			v = logger.DestinationFile

		case "syslog":
			v = logger.DestinationSyslog

		default:
			return fmt.Errorf("invalid log destination: %s", dest)
		}

		if d.contains(v) {
			return fmt.Errorf("log destination set twice")
		}

		*d = append(*d, v)
	}

	return nil
}

// UnmarshalEnv implements envUnmarshaler.
func (d *LogDestinations) UnmarshalEnv(s string) error {
	byts, _ := json.Marshal(strings.Split(s, ","))
	return d.UnmarshalJSON(byts)
}
