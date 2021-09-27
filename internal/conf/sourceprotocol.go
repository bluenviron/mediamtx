package conf

import (
	"encoding/json"
	"fmt"

	"github.com/aler9/gortsplib"
)

// SourceProtocol is the sourceProtocol parameter.
type SourceProtocol struct {
	*gortsplib.ClientProtocol
}

// MarshalJSON marshals a SourceProtocol into JSON.
func (d SourceProtocol) MarshalJSON() ([]byte, error) {
	var out string

	if d.ClientProtocol == nil {
		out = "automatic"
	} else {
		switch *d.ClientProtocol {
		case gortsplib.ClientProtocolUDP:
			out = "udp"

		case gortsplib.ClientProtocolMulticast:
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
		v := gortsplib.ClientProtocolUDP
		d.ClientProtocol = &v

	case "multicast":
		v := gortsplib.ClientProtocolMulticast
		d.ClientProtocol = &v

	case "tcp":
		v := gortsplib.ClientProtocolTCP
		d.ClientProtocol = &v

	case "automatic":

	default:
		return fmt.Errorf("invalid protocol '%s'", in)
	}

	return nil
}
