package conf

import (
	"encoding/json"
	"fmt"

	"github.com/aler9/gortsplib"
)

// SourceProtocol is the sourceProtocol parameter.
type SourceProtocol struct {
	*gortsplib.Transport
}

// MarshalJSON marshals a SourceProtocol into JSON.
func (d SourceProtocol) MarshalJSON() ([]byte, error) {
	var out string

	if d.Transport == nil {
		out = "automatic"
	} else {
		switch *d.Transport {
		case gortsplib.TransportUDP:
			out = "udp"

		case gortsplib.TransportUDPMulticast:
			out = "multicast"

		default:
			out = "tcp"
		}
	}

	return json.Marshal(out)
}

// UnmarshalJSON unmarshals a SourceProtocol from JSON.
func (d *SourceProtocol) UnmarshalJSON(b []byte) error {
	var in string
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}

	switch in {
	case "udp":
		v := gortsplib.TransportUDP
		d.Transport = &v

	case "multicast":
		v := gortsplib.TransportUDPMulticast
		d.Transport = &v

	case "tcp":
		v := gortsplib.TransportTCP
		d.Transport = &v

	case "automatic":

	default:
		return fmt.Errorf("invalid protocol '%s'", in)
	}

	return nil
}

func (d *SourceProtocol) unmarshalEnv(s string) error {
	return d.UnmarshalJSON([]byte(`"` + s + `"`))
}
