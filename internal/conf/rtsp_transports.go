package conf

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// RTSPTransports is the rtspTransports parameter.
type RTSPTransports map[gortsplib.Protocol]struct{}

// MarshalJSON implements json.Marshaler.
func (d RTSPTransports) MarshalJSON() ([]byte, error) {
	out := make([]string, len(d))
	i := 0

	for p := range d {
		var v string

		switch p {
		case gortsplib.ProtocolUDP:
			v = "udp"

		case gortsplib.ProtocolUDPMulticast:
			v = "multicast"

		default:
			v = "tcp"
		}

		out[i] = v
		i++
	}

	sort.Strings(out)

	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *RTSPTransports) UnmarshalJSON(b []byte) error {
	var in []string
	if err := jsonwrapper.Unmarshal(b, &in); err != nil {
		return err
	}

	*d = make(RTSPTransports)

	for _, proto := range in {
		switch proto {
		case "udp":
			(*d)[gortsplib.ProtocolUDP] = struct{}{}

		case "multicast":
			(*d)[gortsplib.ProtocolUDPMulticast] = struct{}{}

		case "tcp":
			(*d)[gortsplib.ProtocolTCP] = struct{}{}

		default:
			return fmt.Errorf("invalid transport: %s", proto)
		}
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *RTSPTransports) UnmarshalEnv(_ string, v string) error {
	byts, _ := json.Marshal(strings.Split(v, ","))
	return d.UnmarshalJSON(byts)
}
