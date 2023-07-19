//go:build rpicamera
// +build rpicamera

package rpicamera

import (
	"debug/elf"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
)

const (
	tempPathPrefix = "/dev/shm/rtspss-embeddedexe-"
)

//go:embed exe/exe
var exeContent []byte

func startEmbeddedExe(content []byte, env []string) (*exec.Cmd, error) {
	tempPath := tempPathPrefix + strconv.FormatInt(time.Now().UnixNano(), 10)

	err := os.WriteFile(tempPath, content, 0o755)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(tempPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	err = cmd.Start()
	os.Remove(tempPath)

	if err != nil {
		return nil, err
	}

	return cmd, nil
}

func findLibrary(name string) (string, error) {
	byts, err := exec.Command("ldconfig", "-p").Output()
	if err == nil {
		for _, line := range strings.Split(string(byts), "\n") {
			f := strings.Split(line, " => ")
			if len(f) == 2 && strings.Contains(f[1], name+".so") {
				return f[1], nil
			}
		}
	}

	return "", fmt.Errorf("library '%s' not found", name)
}

func check64bit(fpath string) error {
	f, err := os.Open(fpath)
	if err != nil {
		return err
	}
	defer f.Close()

	ef, err := elf.NewFile(f)
	if err != nil {
		return err
	}
	defer ef.Close()

	if ef.FileHeader.Class == elf.ELFCLASS64 {
		return fmt.Errorf("libcamera is 64-bit, you need the 64-bit server version")
	}

	return nil
}

func setupSymlink(name string) error {
	lib, err := findLibrary(name)
	if err != nil {
		return err
	}

	if runtime.GOARCH == "arm" {
		err := check64bit(lib)
		if err != nil {
			return err
		}
	}

	os.Remove("/dev/shm/" + name + ".so.x.x.x")
	return os.Symlink(lib, "/dev/shm/"+name+".so.x.x.x")
}

var (
	mutex    sync.Mutex
	setupped bool
)

func setupLibcameraOnce() error {
	mutex.Lock()
	defer mutex.Unlock()

	if !setupped {
		err := setupSymlink("libcamera")
		if err != nil {
			return err
		}

		err = setupSymlink("libcamera-base")
		if err != nil {
			return err
		}

		setupped = true
	}

	return nil
}

// Cleanup cleanups files created by the camera implementation.
func Cleanup() {
	if setupped {
		os.Remove("/dev/shm/libcamera-base.so.x.x.x")
		os.Remove("/dev/shm/libcamera.so.x.x.x")
	}
}

type RPICamera struct {
	onData func(time.Duration, [][]byte)

	cmd       *exec.Cmd
	pipeConf  *pipe
	pipeVideo *pipe

	waitDone   chan error
	readerDone chan error
}

func New(
	params Params,
	onData func(time.Duration, [][]byte),
) (*RPICamera, error) {
	err := setupLibcameraOnce()
	if err != nil {
		return nil, err
	}

	c := &RPICamera{
		onData: onData,
	}

	c.pipeConf, err = newPipe()
	if err != nil {
		return nil, err
	}

	c.pipeVideo, err = newPipe()
	if err != nil {
		c.pipeConf.close()
		return nil, err
	}

	env := []string{
		"LD_LIBRARY_PATH=/dev/shm",
		"PIPE_CONF_FD=" + strconv.FormatInt(int64(c.pipeConf.readFD), 10),
		"PIPE_VIDEO_FD=" + strconv.FormatInt(int64(c.pipeVideo.writeFD), 10),
	}

	c.cmd, err = startEmbeddedExe(exeContent, env)
	if err != nil {
		c.pipeConf.close()
		c.pipeVideo.close()
		return nil, err
	}

	c.pipeConf.write(append([]byte{'c'}, params.serialize()...))

	c.waitDone = make(chan error)
	go func() {
		c.waitDone <- c.cmd.Wait()
	}()

	c.readerDone = make(chan error)
	go func() {
		c.readerDone <- c.readReady()
	}()

	select {
	case <-c.waitDone:
		c.pipeConf.close()
		c.pipeVideo.close()
		<-c.readerDone
		return nil, fmt.Errorf("process exited unexpectedly")

	case err := <-c.readerDone:
		if err != nil {
			c.pipeConf.write([]byte{'e'})
			<-c.waitDone
			c.pipeConf.close()
			c.pipeVideo.close()
			return nil, err
		}
	}

	c.readerDone = make(chan error)
	go func() {
		c.readerDone <- c.readData()
	}()

	return c, nil
}

func (c *RPICamera) Close() {
	c.pipeConf.write([]byte{'e'})
	<-c.waitDone
	c.pipeConf.close()
	c.pipeVideo.close()
	<-c.readerDone
}

func (c *RPICamera) ReloadParams(params Params) {
	c.pipeConf.write(append([]byte{'c'}, params.serialize()...))
}

func (c *RPICamera) readReady() error {
	buf, err := c.pipeVideo.read()
	if err != nil {
		return err
	}

	switch buf[0] {
	case 'e':
		return fmt.Errorf(string(buf[1:]))

	case 'r':
		return nil

	default:
		return fmt.Errorf("unexpected output from video pipe: '0x%.2x'", buf[0])
	}
}

func (c *RPICamera) readData() error {
	for {
		buf, err := c.pipeVideo.read()
		if err != nil {
			return err
		}

		if buf[0] != 'b' {
			return fmt.Errorf("unexpected output from pipe (%c)", buf[0])
		}

		tmp := uint64(buf[8])<<56 | uint64(buf[7])<<48 | uint64(buf[6])<<40 | uint64(buf[5])<<32 |
			uint64(buf[4])<<24 | uint64(buf[3])<<16 | uint64(buf[2])<<8 | uint64(buf[1])
		dts := time.Duration(tmp) * time.Microsecond

		nalus, err := h264.AnnexBUnmarshal(buf[9:])
		if err != nil {
			return err
		}

		c.onData(dts, nalus)
	}
}
