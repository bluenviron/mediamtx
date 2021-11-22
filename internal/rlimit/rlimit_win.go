//go:build windows
// +build windows

package rlimit

func Raise() error {
	return nil
}
