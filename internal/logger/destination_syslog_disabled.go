//go:build darwin || windows

package logger

import "fmt"

func newDestinationSyslog(_ string) (destination, error) {
	return nil, fmt.Errorf("syslog is not available on macOS and Windows")
}
