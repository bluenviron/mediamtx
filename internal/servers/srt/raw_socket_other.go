//go:build !windows

package srt

func rawSocket(fd uintptr) int {
	return int(fd)
}
