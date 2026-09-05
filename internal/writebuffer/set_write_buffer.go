// Package writebuffer contains a function to set the write buffer size of a UDP socket.
package writebuffer

import (
	"syscall"
)

// PacketConn is a packet connection.
type PacketConn interface {
	SyscallConn() (syscall.RawConn, error)
}

// SetWriteBuffer sets the write buffer size of the UDP connection and checks that it was set correctly.
func SetWriteBuffer(pc PacketConn, size int) error {
	rawConn, err := pc.SyscallConn()
	if err != nil {
		return err
	}

	return SetWriteBufferRaw(rawConn, size)
}
