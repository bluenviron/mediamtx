//go:build (linux && arm) || (linux && arm64)

package rpicamera

import (
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
	buf := make([]byte, 4)
	err := syscallReadAll(p.readFD, buf)
	if err != nil {
		return nil, err
	}

	le := int(buf[3])<<24 | int(buf[2])<<16 | int(buf[1])<<8 | int(buf[0])
	buf = make([]byte, le)

	err = syscallReadAll(p.readFD, buf)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

func (p *pipe) write(byts []byte) error {
	le := len(byts)
	_, err := syscall.Write(p.writeFD, []byte{byte(le), byte(le >> 8), byte(le >> 16), byte(le >> 24)})
	if err != nil {
		return err
	}

	_, err = syscall.Write(p.writeFD, byts)
	return err
}
