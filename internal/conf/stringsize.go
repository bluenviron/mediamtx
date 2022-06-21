package conf

import (
	"encoding/json"

	"code.cloudfoundry.org/bytefmt"
)

// StringSize is a size that is unmarshaled from a string.
type StringSize uint64

// MarshalJSON implements json.Marshaler.
func (s StringSize) MarshalJSON() ([]byte, error) {
	return []byte(`"` + bytefmt.ByteSize(uint64(s)) + `"`), nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (s *StringSize) UnmarshalJSON(b []byte) error {
	var in string
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}

	v, err := bytefmt.ToBytes(in)
	if err != nil {
		return err
	}

	*s = StringSize(v)
	return nil
}

func (s *StringSize) unmarshalEnv(v string) error {
	return s.UnmarshalJSON([]byte(`"` + v + `"`))
}
