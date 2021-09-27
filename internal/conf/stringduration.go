package conf

import (
	"encoding/json"
	"time"
)

// StringDuration is a duration that is unmarshaled from a string.
// Durations are normally unmarshaled from numbers.
type StringDuration time.Duration

// MarshalJSON marshals a StringDuration into JSON.
func (d StringDuration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// UnmarshalJSON unmarshals a StringDuration from JSON.
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
