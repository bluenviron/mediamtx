//go:build !windows

package webrtc

func rawSocket(fd uintptr) int {
	return int(fd)
}
