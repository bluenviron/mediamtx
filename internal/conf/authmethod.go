package conf

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aler9/gortsplib/pkg/headers"
)

func unmarshalStringSlice(b []byte) ([]string, error) {
	var in interface{}
	if err := json.Unmarshal(b, &in); err != nil {
		return nil, err
	}

	var slice []string

	switch it := in.(type) {
	case string: // from environment variables
		slice = strings.Split(it, ",")

	case []interface{}: // from yaml
		for _, e := range it {
			et, ok := e.(string)
			if !ok {
				return nil, fmt.Errorf("cannot unmarshal from %T", e)
			}
			slice = append(slice, et)
		}

	default:
		return nil, fmt.Errorf("cannot unmarshal from %T", in)
	}

	return slice, nil
}

// AuthMethods is the authMethods parameter.
type AuthMethods []headers.AuthMethod

// MarshalJSON marshals a AuthMethods into JSON.
func (d AuthMethods) MarshalJSON() ([]byte, error) {
	var out []string

	for _, v := range d {
		switch v {
		case headers.AuthBasic:
			out = append(out, "basic")

		default:
			out = append(out, "digest")
		}
	}

	return json.Marshal(out)
}

// UnmarshalJSON unmarshals a AuthMethods from JSON.
func (d *AuthMethods) UnmarshalJSON(b []byte) error {
	slice, err := unmarshalStringSlice(b)
	if err != nil {
		return err
	}

	for _, v := range slice {
		switch v {
		case "basic":
			*d = append(*d, headers.AuthBasic)

		case "digest":
			*d = append(*d, headers.AuthDigest)

		default:
			return fmt.Errorf("invalid authentication method: %s", v)
		}
	}

	return nil
}
