package conf

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// IPNetwork represents an IP network.
type IPNetwork net.IPNet

// MarshalJSON implements json.Marshaler.
func (n IPNetwork) MarshalJSON() ([]byte, error) {
	return json.Marshal(n.String())
}

// UnmarshalJSON implements json.Unmarshaler.
func (n *IPNetwork) UnmarshalJSON(b []byte) error {
	var t string
	if err := jsonwrapper.Unmarshal(b, &t); err != nil {
		return err
	}

	if _, ipnet, err := net.ParseCIDR(t); err == nil {
		if ipv4 := ipnet.IP.To4(); ipv4 != nil {
			*n = IPNetwork{IP: ipv4, Mask: ipnet.Mask[len(ipnet.Mask)-4 : len(ipnet.Mask)]}
		} else {
			*n = IPNetwork(*ipnet)
		}
	} else if ip := net.ParseIP(t); ip != nil {
		if ipv4 := ip.To4(); ipv4 != nil {
			*n = IPNetwork{IP: ipv4, Mask: net.CIDRMask(32, 32)}
		} else {
			*n = IPNetwork{IP: ip, Mask: net.CIDRMask(128, 128)}
		}
	} else {
		return fmt.Errorf("unable to parse IP/CIDR '%s'", t)
	}

	return nil
}

// String implements fmt.Stringer.
func (n IPNetwork) String() string {
	ipnet := net.IPNet(n)
	return ipnet.String()
}

// Contains checks whether the IP is part of the network.
func (n IPNetwork) Contains(ip net.IP) bool {
	ipnet := net.IPNet(n)
	return ipnet.Contains(ip)
}
