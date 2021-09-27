package conf

import (
	"encoding/json"
	"fmt"

	"github.com/aler9/gortsplib/pkg/headers"
)

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
	var in []string
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}

	for _, v := range in {
		switch v {
		case "basic":
			*d = append(*d, headers.AuthBasic)

		case "digest":
			*d = append(*d, headers.AuthDigest)

		default:
			return fmt.Errorf("unsupported authentication method: %s", in)
		}
	}

	return nil
}
