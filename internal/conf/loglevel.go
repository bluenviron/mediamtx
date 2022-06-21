package conf

import (
	"encoding/json"
	"fmt"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

// LogLevel is the logLevel parameter.
type LogLevel logger.Level

// MarshalJSON implements json.Marshaler.
func (d LogLevel) MarshalJSON() ([]byte, error) {
	var out string

	switch d {
	case LogLevel(logger.Error):
		out = "error"

	case LogLevel(logger.Warn):
		out = "warn"

	case LogLevel(logger.Info):
		out = "info"

	default:
		out = "debug"
	}

	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *LogLevel) UnmarshalJSON(b []byte) error {
	var in string
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}

	switch in {
	case "error":
		*d = LogLevel(logger.Error)

	case "warn":
		*d = LogLevel(logger.Warn)

	case "info":
		*d = LogLevel(logger.Info)

	case "debug":
		*d = LogLevel(logger.Debug)

	default:
		return fmt.Errorf("invalid log level: %s", in)
	}

	return nil
}

func (d *LogLevel) unmarshalEnv(s string) error {
	return d.UnmarshalJSON([]byte(`"` + s + `"`))
}
