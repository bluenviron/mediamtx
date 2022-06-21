package core

import (
	"fmt"
	"net"
)

func ipEqualOrInRange(ip net.IP, ips []fmt.Stringer) bool {
	for _, item := range ips {
		switch titem := item.(type) {
		case net.IP:
			if titem.Equal(ip) {
				return true
			}

		case *net.IPNet:
			if titem.Contains(ip) {
				return true
			}
		}
	}
	return false
}
