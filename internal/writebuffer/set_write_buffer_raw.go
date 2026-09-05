package writebuffer

import "syscall"

// SetWriteBufferRaw sets the write buffer size of the raw UDP connection and checks that it was set correctly.
func SetWriteBufferRaw(rc syscall.RawConn, size int) error {
	var err2 error

	err := rc.Control(func(fd uintptr) {
		err2 = setWriteBufferRawInControl(fd, size)
	})
	if err != nil {
		return err
	}

	return err2
}
