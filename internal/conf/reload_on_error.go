package conf

import (
	"encoding/json"
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// ReloadOnError is the reloadOnError parameter.
type ReloadOnError int

const (
	// ReloadOnErrorExit exits the process on config reload error (default, original behavior).
	ReloadOnErrorExit ReloadOnError = iota
	// ReloadOnErrorContinue keeps running with the current configuration on config reload error.
	ReloadOnErrorContinue
)

// MarshalJSON implements json.Marshaler.
func (d ReloadOnError) MarshalJSON() ([]byte, error) {
	var out string

	switch d {
	case ReloadOnErrorContinue:
		out = "continue"
	default:
		out = "exit"
	}

	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *ReloadOnError) UnmarshalJSON(b []byte) error {
	var in string
	if err := jsonwrapper.Unmarshal(b, &in); err != nil {
		return err
	}

	switch in {
	case "exit":
		*d = ReloadOnErrorExit
	case "continue":
		*d = ReloadOnErrorContinue
	default:
		return fmt.Errorf("invalid reloadOnError value: '%s' (valid: exit, continue)", in)
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *ReloadOnError) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}
