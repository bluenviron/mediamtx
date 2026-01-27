package conf

import (
	"encoding/json"
	"strings"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// LogDestinations is the logDestionations parameter.
type LogDestinations []LogDestination

// UnmarshalEnv implements env.Unmarshaler.
func (d *LogDestinations) UnmarshalEnv(_ string, v string) error {
	byts, _ := json.Marshal(strings.Split(v, ","))
	return jsonwrapper.Unmarshal(byts, d)
}

// ToDestinations converts to logger.Destination slice.
func (d LogDestinations) ToDestinations() []logger.Destination {
	out := make([]logger.Destination, len(d))
	for i, v := range d {
		out[i] = logger.Destination(v)
	}
	return out
}
