package writebuffer

import (
	"fmt"
	"syscall"
)

func setWriteBufferRawInControl(fd uintptr, size int) error {
	err := syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF, size)
	if err != nil {
		return err
	}

	v, err := syscall.GetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF)
	if err != nil {
		return err
	}

	if v != size {
		return fmt.Errorf("unable to set UDP write buffer size to %d, got %d, check that the operating system allows that",
			size, v)
	}

	return nil
}
