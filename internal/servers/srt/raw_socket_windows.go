package srt

import "syscall"

func rawSocket(fd uintptr) syscall.Handle {
	return syscall.Handle(fd)
}
