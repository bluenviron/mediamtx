package conf

import (
	"encoding/json"
	"fmt"
	"net"
)

// IPsOrNets is a parameter that acceps IPs or subnets.
type IPsOrNets []interface{}

// MarshalJSON marshals a IPsOrNets into JSON.
func (d IPsOrNets) MarshalJSON() ([]byte, error) {
	out := make([]string, len(d))

	for i, v := range d {
		out[i] = v.(fmt.Stringer).String()
	}

	return json.Marshal(out)
}

// UnmarshalJSON unmarshals a IPsOrNets from JSON.
func (d *IPsOrNets) UnmarshalJSON(b []byte) error {
	slice, err := unmarshalStringSlice(b)
	if err != nil {
		return err
	}

	if len(slice) == 0 {
		return nil
	}

	for _, t := range slice {
		if _, ipnet, err := net.ParseCIDR(t); err == nil {
			*d = append(*d, ipnet)
		} else if ip := net.ParseIP(t); ip != nil {
			*d = append(*d, ip)
		} else {
			return fmt.Errorf("unable to parse ip/network '%s'", t)
		}
	}

	return nil
}
