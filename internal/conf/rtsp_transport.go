package conf

import (
	"encoding/json"
	"fmt"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// RTSPTransport is the rtspTransport parameter.
type RTSPTransport struct {
	*gortsplib.Protocol
}

// MarshalJSON implements json.Marshaler.
func (d RTSPTransport) MarshalJSON() ([]byte, error) {
	var out string

	if d.Protocol == nil {
		out = "automatic"
	} else {
		switch *d.Protocol {
		case gortsplib.ProtocolUDP:
			out = "udp"

		case gortsplib.ProtocolUDPMulticast:
			out = "multicast"

		default:
			out = "tcp"
		}
	}

	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *RTSPTransport) UnmarshalJSON(b []byte) error {
	var in string
	if err := jsonwrapper.Unmarshal(b, &in); err != nil {
		return err
	}

	switch in {
	case "udp":
		v := gortsplib.ProtocolUDP
		d.Protocol = &v

	case "multicast":
		v := gortsplib.ProtocolUDPMulticast
		d.Protocol = &v

	case "tcp":
		v := gortsplib.ProtocolTCP
		d.Protocol = &v

	case "automatic":
		d.Protocol = nil

	default:
		return fmt.Errorf("invalid transport '%s'", in)
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *RTSPTransport) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}
