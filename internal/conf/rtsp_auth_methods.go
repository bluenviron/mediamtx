package conf

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/bluenviron/gortsplib/v4/pkg/headers"
)

// RTSPAuthMethods is the rtspAuthMethods parameter.
type RTSPAuthMethods []headers.AuthMethod

// MarshalJSON implements json.Marshaler.
func (d RTSPAuthMethods) MarshalJSON() ([]byte, error) {
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
func (d *RTSPAuthMethods) UnmarshalJSON(b []byte) error {
	var in []string
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}

	*d = nil

	for _, v := range in {
		switch v {
		case "basic":
			*d = append(*d, headers.AuthBasic)

		case "digest":
			*d = append(*d, headers.AuthDigestMD5)

		default:
			return fmt.Errorf("invalid authentication method: '%s'", v)
		}
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *RTSPAuthMethods) UnmarshalEnv(_ string, v string) error {
	byts, _ := json.Marshal(strings.Split(v, ","))
	return d.UnmarshalJSON(byts)
}
