//go:build (linux && arm) || (linux && arm64)

package rpicamera

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
	"unsafe"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
)

func ntpTime() syscall.Timespec {
	var t syscall.Timespec
	syscall.Syscall(syscall.SYS_CLOCK_GETTIME, 0, uintptr(unsafe.Pointer(&t)), 0)
	return t
}

func monotonicTime() syscall.Timespec {
	var t syscall.Timespec
	syscall.Syscall(syscall.SYS_CLOCK_GETTIME, 1, uintptr(unsafe.Pointer(&t)), 0)
	return t
}

func multiplyAndDivide(v, m, d int64) int64 {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

type camera struct {
	params params
	onData func(int64, time.Time, [][]byte)

	cmd      *exec.Cmd
	pipeOut  *pipe
	pipeIn   *pipe
	finalErr error

	terminate chan struct{}
	done      chan struct{}
}

func (c *camera) initialize() error {
	err := dumpComponent()
	if err != nil {
		return err
	}

	c.pipeOut, err = newPipe()
	if err != nil {
		freeComponent()
		return err
	}

	c.pipeIn, err = newPipe()
	if err != nil {
		c.pipeOut.close()
		freeComponent()
		return err
	}

	env := []string{
		"PIPE_CONF_FD=" + strconv.FormatInt(int64(c.pipeOut.readFD), 10),
		"PIPE_VIDEO_FD=" + strconv.FormatInt(int64(c.pipeIn.writeFD), 10),
		"LD_LIBRARY_PATH=" + dumpPath,
	}

	c.cmd = exec.Command(filepath.Join(dumpPath, executableName))
	c.cmd.Stdout = os.Stdout
	c.cmd.Stderr = os.Stderr
	c.cmd.Env = env
	c.cmd.Dir = dumpPath

	err = c.cmd.Start()
	if err != nil {
		c.pipeOut.close()
		c.pipeIn.close()
		freeComponent()
		return err
	}

	c.terminate = make(chan struct{})
	c.done = make(chan struct{})

	go c.run()

	c.pipeOut.write(append([]byte{'c'}, c.params.serialize()...))

	return nil
}

func (c *camera) close() {
	close(c.terminate)
	<-c.done
	freeComponent()
}

func (c *camera) run() {
	defer close(c.done)
	c.finalErr = c.runInner()
}

func (c *camera) runInner() error {
	cmdDone := make(chan error)
	go func() {
		cmdDone <- c.cmd.Wait()
	}()

	readDone := make(chan error)
	go func() {
		readDone <- c.runReader()
	}()

	for {
		select {
		case err := <-cmdDone:
			c.pipeIn.close()
			c.pipeOut.close()

			<-readDone

			return err

		case err := <-readDone:
			c.pipeIn.close()

			c.pipeOut.write([]byte{'e'})
			c.pipeOut.close()

			<-cmdDone

			return err

		case <-c.terminate:
			c.pipeIn.close()
			<-readDone

			c.pipeOut.write([]byte{'e'})
			c.pipeOut.close()

			<-cmdDone

			return fmt.Errorf("terminated")
		}
	}
}

func (c *camera) runReader() error {
outer:
	for {
		buf, err := c.pipeIn.read()
		if err != nil {
			return err
		}

		switch buf[0] {
		case 'e':
			return fmt.Errorf(string(buf[1:]))

		case 'r':
			break outer

		default:
			return fmt.Errorf("unexpected data from pipe: '0x%.2x'", buf[0])
		}
	}

	for {
		buf, err := c.pipeIn.read()
		if err != nil {
			return err
		}

		switch buf[0] {
		case 'e':
			return fmt.Errorf(string(buf[1:]))

		case 'b':
			dts := int64(buf[8])<<56 | int64(buf[7])<<48 | int64(buf[6])<<40 | int64(buf[5])<<32 |
				int64(buf[4])<<24 | int64(buf[3])<<16 | int64(buf[2])<<8 | int64(buf[1])

			var nalus h264.AnnexB
			err = nalus.Unmarshal(buf[9:])
			if err != nil {
				return err
			}

			unixNTP := ntpTime()
			unixMono := monotonicTime()

			// subtract from NTP the delay from now to the moment the frame was taken
			ntp := time.Unix(int64(unixNTP.Sec), int64(unixNTP.Nsec))
			deltaT := time.Duration(unixMono.Nano()-dts*1e3) * time.Nanosecond
			ntp = ntp.Add(-deltaT)

			c.onData(
				multiplyAndDivide(dts, 90000, 1e6),
				ntp,
				nalus)

		default:
			return fmt.Errorf("unexpected data from pipe: '0x%.2x'", buf[0])
		}
	}
}

func (c *camera) reloadParams(params params) {
	c.pipeOut.write(append([]byte{'c'}, params.serialize()...))
}

func (c *camera) wait() error {
	<-c.done
	return c.finalErr
}
