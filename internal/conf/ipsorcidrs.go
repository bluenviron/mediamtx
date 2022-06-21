package conf

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"
)

// IPsOrCIDRs is a parameter that contains a list of IPs or CIDRs.
type IPsOrCIDRs []fmt.Stringer

// MarshalJSON implements json.Marshaler.
func (d IPsOrCIDRs) MarshalJSON() ([]byte, error) {
	out := make([]string, len(d))

	for i, v := range d {
		out[i] = v.String()
	}

	sort.Strings(out)

	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *IPsOrCIDRs) UnmarshalJSON(b []byte) error {
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
			return fmt.Errorf("unable to parse IP/CIDR '%s'", t)
		}
	}

	return nil
}

func (d *IPsOrCIDRs) unmarshalEnv(s string) error {
	byts, _ := json.Marshal(strings.Split(s, ","))
	return d.UnmarshalJSON(byts)
}
