package conf

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"
)

// IPsOrNets is a parameter that acceps IPs or subnets.
type IPsOrNets []interface{}

// MarshalJSON marshals a IPsOrNets into JSON.
func (d IPsOrNets) MarshalJSON() ([]byte, error) {
	out := make([]string, len(d))

	for i, v := range d {
		out[i] = v.(fmt.Stringer).String()
	}

	sort.Strings(out)

	return json.Marshal(out)
}

// UnmarshalJSON unmarshals a IPsOrNets from JSON.
func (d *IPsOrNets) UnmarshalJSON(b []byte) error {
	var in []string
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}

	if len(in) == 0 {
		return nil
	}

	for _, t := range in {
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

func (d *IPsOrNets) unmarshalEnv(s string) error {
	byts, _ := json.Marshal(strings.Split(s, ","))
	return d.UnmarshalJSON(byts)
}
