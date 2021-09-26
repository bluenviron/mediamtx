package conf

import (
	"encoding/json"
	"errors"
	"time"

	"gopkg.in/yaml.v2"
)

// StringDuration is a duration that is unmarshaled from a string.
// Durations are normally unmarshaled from numbers.
type StringDuration time.Duration

// MarshalJSON marshals a StringDuration into a string.
func (d StringDuration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// UnmarshalJSON unmarshals a StringDuration from a string.
func (d *StringDuration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	value, ok := v.(string)
	if !ok {
		return errors.New("invalid duration")
	}

	du, err := time.ParseDuration(value)
	if err != nil {
		return err
	}

	*d = StringDuration(du)
	return nil
}

// MarshalYAML marshals a StringDuration into a string.
func (d StringDuration) MarshalYAML() ([]byte, error) {
	return yaml.Marshal(time.Duration(d).String())
}

// UnmarshalYAML unmarshals a StringDuration from a string.
func (d *StringDuration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var v interface{}
	if err := unmarshal(&v); err != nil {
		return err
	}

	value, ok := v.(string)
	if !ok {
		return errors.New("invalid duration")
	}

	du, err := time.ParseDuration(value)
	if err != nil {
		return err
	}

	*d = StringDuration(du)
	return nil
}
