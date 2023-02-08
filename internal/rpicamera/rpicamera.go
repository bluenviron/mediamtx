//go:build rpicamera
// +build rpicamera

package rpicamera

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/codecs/h264"
)

const (
	tempPathPrefix = "/dev/shm/rtspss-embeddedexe-"
)

//go:embed exe/exe
var exeContent []byte

func getKernelArch() (string, error) {
	cmd := exec.Command("uname", "-m")

	byts, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(byts[:len(byts)-1]), nil
}

// 32-bit embedded executables can't run on 64-bit.
func checkArch() error {
	if runtime.GOARCH != "arm" {
		return nil
	}

	arch, err := getKernelArch()
	if err != nil {
		return err
	}

	if arch == "aarch64" {
		return fmt.Errorf("OS is 64-bit, you need the arm64 server version")
	}

	return nil
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

func setupSymlink(name string) error {
	lib, err := findLibrary(name)
	if err != nil {
		return err
	}

	os.Remove("/dev/shm/" + name + ".so.x.x.x")
	return os.Symlink(lib, "/dev/shm/"+name+".so.x.x.x")
}

// create libcamera simlinks that are version agnostic.
func setupSymlinks() error {
	err := setupSymlink("libcamera")
	if err != nil {
		return err
	}

	return setupSymlink("libcamera-base")
}

func removeSymlinks() {
	os.Remove("/dev/shm/libcamera-base.so.x.x.x")
	os.Remove("/dev/shm/libcamera.so.x.x.x")
}

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

func serializeParams(p Params) []byte {
	rv := reflect.ValueOf(p)
	rt := rv.Type()
	nf := rv.NumField()
	ret := make([]string, nf)

	for i := 0; i < nf; i++ {
		entry := rt.Field(i).Name + "="
		f := rv.Field(i)

		switch f.Kind() {
		case reflect.Int:
			entry += strconv.FormatInt(f.Int(), 10)

		case reflect.Float64:
			entry += strconv.FormatFloat(f.Float(), 'f', -1, 64)

		case reflect.String:
			entry += f.String()

		case reflect.Bool:
			if f.Bool() {
				entry += "1"
			} else {
				entry += "0"
			}

		default:
			panic("unhandled type")
		}

		ret[i] = entry
	}

	return []byte(strings.Join(ret, " "))
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
	err := checkArch()
	if err != nil {
		return nil, err
	}

	err = setupSymlinks()
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
		removeSymlinks()
		c.pipeConf.close()
		c.pipeVideo.close()
		return nil, err
	}

	removeSymlinks()

	c.pipeConf.write(append([]byte{'c'}, serializeParams(params)...))

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
	c.pipeConf.write(append([]byte{'c'}, serializeParams(params)...))
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
