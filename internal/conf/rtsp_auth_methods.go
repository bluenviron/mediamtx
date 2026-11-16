package conf

import (
	"encoding/json"
	"strings"

	"github.com/bluenviron/gortsplib/v5/pkg/auth"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// RTSPAuthMethods is the rtspAuthMethods parameter.
type RTSPAuthMethods []RTSPAuthMethod

// UnmarshalEnv implements env.Unmarshaler.
func (d *RTSPAuthMethods) UnmarshalEnv(_ string, v string) error {
	if v == "" {
		*d = []RTSPAuthMethod{}
		return nil
	}

	byts, _ := json.Marshal(strings.Split(v, ","))
	return jsonwrapper.Unmarshal(byts, d)
}

// ToAuthMethods converts to auth.Method slice.
func (d RTSPAuthMethods) ToAuthMethods() []auth.Method {
	out := make([]auth.Method, len(d))
	for i, v := range d {
		out[i] = auth.Method(v)
	}
	return out
}
