package conf

import (
	"encoding/json"
	"time"
)

// StringDuration is a duration that is unmarshaled from a string.
// Durations are normally unmarshaled from numbers.
type StringDuration time.Duration

// MarshalJSON implements json.Marshaler.
func (d StringDuration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *StringDuration) UnmarshalJSON(b []byte) error {
	var in string
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}

	du, err := time.ParseDuration(in)
	if err != nil {
		return err
	}
	*d = StringDuration(du)

	return nil
}

// UnmarshalEnv implements envUnmarshaler.
func (d *StringDuration) UnmarshalEnv(s string) error {
	return d.UnmarshalJSON([]byte(`"` + s + `"`))
}
