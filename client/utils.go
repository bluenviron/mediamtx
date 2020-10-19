package client

import (
	"fmt"
	"net"
	"strings"
)

func ipEqualOrInRange(ip net.IP, ips []interface{}) bool {
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

func removeQueryFromPath(path string) string {
	i := strings.Index(path, "?")
	if i >= 0 {
		return path[:i]
	}
	return path
}

func splitPathIntoBaseAndControl(path string) (string, string, error) {
	pos := func() int {
		for i := len(path) - 1; i >= 0; i-- {
			if path[i] == '/' {
				return i
			}
		}
		return -1
	}()

	if pos < 0 {
		return "", "", fmt.Errorf("the path must contain a base path and a control path (%s)", path)
	}

	basePath := path[:pos]
	controlPath := path[pos+1:]

	if len(basePath) == 0 {
		return "", "", fmt.Errorf("empty base path (%s)", basePath)
	}

	if len(controlPath) == 0 {
		return "", "", fmt.Errorf("empty control path (%s)", controlPath)
	}

	return basePath, controlPath, nil
}
