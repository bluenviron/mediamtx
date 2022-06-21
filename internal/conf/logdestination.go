package conf

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

// LogDestinations is the logDestionations parameter.
type LogDestinations map[logger.Destination]struct{}

// MarshalJSON implements json.Marshaler.
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

	sort.Strings(out)

	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *LogDestinations) UnmarshalJSON(b []byte) error {
	var in []string
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}

	*d = make(LogDestinations)

	for _, proto := range in {
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

func (d *LogDestinations) unmarshalEnv(s string) error {
	byts, _ := json.Marshal(strings.Split(s, ","))
	return d.UnmarshalJSON(byts)
}
