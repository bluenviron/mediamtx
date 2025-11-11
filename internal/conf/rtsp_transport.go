package conf

import (
	"encoding/json"
	"fmt"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

func ptrOf[T any](v T) *T {
	return &v
}

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
		d.Protocol = ptrOf(gortsplib.ProtocolUDP)

	case "multicast":
		d.Protocol = ptrOf(gortsplib.ProtocolUDPMulticast)

	case "tcp":
		d.Protocol = ptrOf(gortsplib.ProtocolTCP)

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
