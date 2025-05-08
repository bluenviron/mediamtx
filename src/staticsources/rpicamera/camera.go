//go:build (linux && arm) || (linux && arm64)

package rpicamera

import (
	"debug/elf"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
)

const (
	libraryToCheckArchitecture = "libc.so.6"
	dumpPrefix                 = "/dev/shm/mediamtx-rpicamera-"
	executableName             = "mtxrpicam"
)

var (
	dumpMutex sync.Mutex
	dumpCount = 0
	dumpPath  = ""
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

func getArchitecture(libPath string) (bool, error) {
	f, err := os.Open(libPath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	ef, err := elf.NewFile(f)
	if err != nil {
		return false, err
	}
	defer ef.Close()

	return (ef.FileHeader.Class == elf.ELFCLASS64), nil
}

func checkArchitecture() error {
	byts, err := exec.Command("/sbin/ldconfig", "-p").Output()
	if err != nil {
		return fmt.Errorf("ldconfig failed: %w", err)
	}

	for _, line := range strings.Split(string(byts), "\n") {
		f := strings.Split(line, " => ")
		if len(f) == 2 && strings.Contains(f[1], libraryToCheckArchitecture) {
			is64, err := getArchitecture(f[1])
			if err != nil {
				return err
			}

			if runtime.GOARCH == "arm" {
				if !is64 {
					return nil
				}
			} else {
				if is64 {
					return nil
				}
			}
		}
	}

	if runtime.GOARCH == "arm" {
		return fmt.Errorf("the operating system is 64-bit, you need the 64-bit server version")
	}

	return fmt.Errorf("the operating system is 32-bit, you need the 32-bit server version")
}

func dumpEmbedFSRecursive(src string, dest string) error {
	files, err := mtxrpicam.ReadDir(src)
	if err != nil {
		return err
	}

	for _, f := range files {
		if f.IsDir() {
			err = os.Mkdir(filepath.Join(dest, f.Name()), 0o755)
			if err != nil {
				return err
			}

			err = dumpEmbedFSRecursive(filepath.Join(src, f.Name()), filepath.Join(dest, f.Name()))
			if err != nil {
				return err
			}
		} else {
			buf, err := mtxrpicam.ReadFile(filepath.Join(src, f.Name()))
			if err != nil {
				return err
			}

			err = os.WriteFile(filepath.Join(dest, f.Name()), buf, 0o644)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func dumpComponent() error {
	dumpMutex.Lock()
	defer dumpMutex.Unlock()

	if dumpCount > 0 {
		dumpCount++
		return nil
	}

	err := checkArchitecture()
	if err != nil {
		return err
	}

	dumpPath = dumpPrefix + strconv.FormatInt(time.Now().UnixNano(), 10)

	err = os.Mkdir(dumpPath, 0o755)
	if err != nil {
		return err
	}

	files, err := mtxrpicam.ReadDir(".")
	if err != nil {
		os.RemoveAll(dumpPath)
		return err
	}

	err = dumpEmbedFSRecursive(files[0].Name(), dumpPath)
	if err != nil {
		os.RemoveAll(dumpPath)
		return err
	}

	err = os.Chmod(filepath.Join(dumpPath, executableName), 0o755)
	if err != nil {
		os.RemoveAll(dumpPath)
		return err
	}

	dumpCount++

	return nil
}

func freeComponent() {
	dumpMutex.Lock()
	defer dumpMutex.Unlock()

	dumpCount--

	if dumpCount == 0 {
		os.RemoveAll(dumpPath)
	}
}

type camera struct {
	params          params
	onData          func(int64, time.Time, [][]byte)
	onDataSecondary func(int64, time.Time, []byte)

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

		case 'd':
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

		case 's':
			dts := int64(buf[8])<<56 | int64(buf[7])<<48 | int64(buf[6])<<40 | int64(buf[5])<<32 |
				int64(buf[4])<<24 | int64(buf[3])<<16 | int64(buf[2])<<8 | int64(buf[1])

			unixNTP := ntpTime()
			unixMono := monotonicTime()

			// subtract from NTP the delay from now to the moment the frame was taken
			ntp := time.Unix(int64(unixNTP.Sec), int64(unixNTP.Nsec))
			deltaT := time.Duration(unixMono.Nano()-dts*1e3) * time.Nanosecond
			ntp = ntp.Add(-deltaT)

			c.onDataSecondary(
				multiplyAndDivide(dts, 90000, 1e6),
				ntp,
				buf[9:])

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
