//go:build !windows
// +build !windows

package rlimit

import (
	"syscall"
)

// Raise raises the number of file descriptors that can be opened.
func Raise() error {
	var rlim syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlim)
	if err != nil {
		return err
	}

	rlim.Cur = 999999
	err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rlim)
	if err != nil {
		return err
	}

	err = syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlim)
	if err != nil {
		return err
	}

	return nil
}
