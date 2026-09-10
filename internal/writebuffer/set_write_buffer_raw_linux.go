package writebuffer

import (
	"fmt"
	"syscall"
)

func setWriteBufferRawInControl(fd uintptr, size int) error {
	err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF, size)
	if err != nil {
		return err
	}

	v, err := syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF)
	if err != nil {
		return err
	}

	v /= 2 // Linux doubles the value set with SO_SNDBUF

	if v != size {
		return fmt.Errorf("unable to set UDP write buffer size to %d, got %d, check that the operating system allows that",
			size, v)
	}

	return nil
}
