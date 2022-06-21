package conf

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/aler9/gortsplib"
)

// Protocol is a RTSP transport.
type Protocol gortsplib.Transport

// Protocols is the protocols parameter.
type Protocols map[Protocol]struct{}

// MarshalJSON implements json.Marshaler.
func (d Protocols) MarshalJSON() ([]byte, error) {
	out := make([]string, len(d))
	i := 0

	for p := range d {
		var v string

		switch p {
		case Protocol(gortsplib.TransportUDP):
			v = "udp"

		case Protocol(gortsplib.TransportUDPMulticast):
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
func (d *Protocols) UnmarshalJSON(b []byte) error {
	var in []string
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}

	*d = make(Protocols)

	for _, proto := range in {
		switch proto {
		case "udp":
			(*d)[Protocol(gortsplib.TransportUDP)] = struct{}{}

		case "multicast":
			(*d)[Protocol(gortsplib.TransportUDPMulticast)] = struct{}{}

		case "tcp":
			(*d)[Protocol(gortsplib.TransportTCP)] = struct{}{}

		default:
			return fmt.Errorf("invalid protocol: %s", proto)
		}
	}

	return nil
}

func (d *Protocols) unmarshalEnv(s string) error {
	byts, _ := json.Marshal(strings.Split(s, ","))
	return d.UnmarshalJSON(byts)
}
