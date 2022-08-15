//go:build rpicamera
// +build rpicamera

package rpicamera

import (
	"encoding/binary"
	"syscall"
)

func syscallReadAll(fd int, buf []byte) error {
	size := len(buf)
	read := 0

	for {
		n, err := syscall.Read(fd, buf[read:size])
		if err != nil {
			return err
		}

		read += n
		if read >= size {
			break
		}
	}

	return nil
}

type pipe struct {
	readFD  int
	writeFD int
}

func newPipe() (*pipe, error) {
	fds := make([]int, 2)
	err := syscall.Pipe(fds)
	if err != nil {
		return nil, err
	}

	return &pipe{
		readFD:  fds[0],
		writeFD: fds[1],
	}, nil
}

func (p *pipe) close() {
	syscall.Close(p.readFD)
	syscall.Close(p.writeFD)
}

func (p *pipe) read() ([]byte, error) {
	sizebuf := make([]byte, 4)
	err := syscallReadAll(p.readFD, sizebuf)
	if err != nil {
		return nil, err
	}

	size := int(binary.LittleEndian.Uint32(sizebuf))
	buf := make([]byte, size)

	err = syscallReadAll(p.readFD, buf)
	if err != nil {
		return nil, err
	}

	return buf, nil
}
