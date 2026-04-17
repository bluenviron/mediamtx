package conf

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// ListenerFailurePolicy controls what happens when a protocol listener
// fails to start (e.g. its port is already in use).
type ListenerFailurePolicy string

// values.
const (
	ListenerFailurePolicyFatal ListenerFailurePolicy = "fatal"
	ListenerFailurePolicyWarn  ListenerFailurePolicy = "warn"
)

// UnmarshalJSON implements json.Unmarshaler.
func (d *ListenerFailurePolicy) UnmarshalJSON(b []byte) error {
	type alias ListenerFailurePolicy
	if err := jsonwrapper.Unmarshal(b, (*alias)(d)); err != nil {
		return err
	}

	switch *d {
	case ListenerFailurePolicyFatal, ListenerFailurePolicyWarn:

	default:
		return fmt.Errorf("invalid listener failure policy: '%s'", *d)
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *ListenerFailurePolicy) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}
