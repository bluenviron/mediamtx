package conf

import (
	"encoding/json"
	"net"
	"strings"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// IPNetworks is a parameter that contains a list of IP networks.
type IPNetworks []IPNetwork

// UnmarshalEnv implements env.Unmarshaler.
func (d *IPNetworks) UnmarshalEnv(_ string, v string) error {
	byts, _ := json.Marshal(strings.Split(v, ","))
	return jsonwrapper.Unmarshal(byts, d)
}

// ToTrustedProxies converts IPNetworks into a string slice for SetTrustedProxies.
func (d *IPNetworks) ToTrustedProxies() []string {
	ret := make([]string, len(*d))
	for i, entry := range *d {
		ret[i] = entry.String()
	}
	return ret
}

// Contains checks whether the IP is part of one of the networks.
func (d IPNetworks) Contains(ip net.IP) bool {
	for _, network := range d {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
