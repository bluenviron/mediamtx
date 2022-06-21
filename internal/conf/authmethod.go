package conf

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/aler9/gortsplib/pkg/headers"
)

// AuthMethods is the authMethods parameter.
type AuthMethods []headers.AuthMethod

// MarshalJSON implements json.Marshaler.
func (d AuthMethods) MarshalJSON() ([]byte, error) {
	out := make([]string, len(d))

	for i, v := range d {
		switch v {
		case headers.AuthBasic:
			out[i] = "basic"

		default:
			out[i] = "digest"
		}
	}

	sort.Strings(out)

	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
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
			return fmt.Errorf("invalid authentication method: %s", v)
		}
	}

	return nil
}

func (d *AuthMethods) unmarshalEnv(s string) error {
	byts, _ := json.Marshal(strings.Split(s, ","))
	return d.UnmarshalJSON(byts)
}
