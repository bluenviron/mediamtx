package conf

import (
	"encoding/json"
	"fmt"

	"github.com/bluenviron/gortsplib/v5/pkg/auth"
	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// RTSPAuthMethod represents an RTSP authentication method.
type RTSPAuthMethod auth.VerifyMethod

// MarshalJSON implements json.Marshaler.
func (d RTSPAuthMethod) MarshalJSON() ([]byte, error) {
	switch d {
	case RTSPAuthMethod(auth.VerifyMethodBasic):
		return json.Marshal("basic")

	default:
		return json.Marshal("digest")
	}
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *RTSPAuthMethod) UnmarshalJSON(b []byte) error {
	var in string
	if err := jsonwrapper.Unmarshal(b, &in); err != nil {
		return err
	}

	switch in {
	case "basic":
		*d = RTSPAuthMethod(auth.VerifyMethodBasic)

	case "digest":
		*d = RTSPAuthMethod(auth.VerifyMethodDigestMD5)

	default:
		return fmt.Errorf("invalid authentication method: '%s'", in)
	}

	return nil
}
