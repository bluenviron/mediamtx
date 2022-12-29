//go:build rpicamera
// +build rpicamera

package rpicamera

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/codecs/h264"
)

//go:embed exe/exe
var exeContent []byte

func bool2env(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

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

type RPICamera struct {
	onData func(time.Duration, [][]byte)

	exe  *embeddedExe
	pipe *pipe

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

	pipe, err := newPipe()
	if err != nil {
		return nil, err
	}

	env := []string{
		"LD_LIBRARY_PATH=/dev/shm",
		"PIPE_FD=" + strconv.FormatInt(int64(pipe.writeFD), 10),
		"CAMERA_ID=" + strconv.FormatInt(int64(params.CameraID), 10),
		"WIDTH=" + strconv.FormatInt(int64(params.Width), 10),
		"HEIGHT=" + strconv.FormatInt(int64(params.Height), 10),
		"H_FLIP=" + bool2env(params.HFlip),
		"V_FLIP=" + bool2env(params.VFlip),
		"BRIGHTNESS=" + strconv.FormatFloat(params.Brightness, 'f', -1, 64),
		"CONTRAST=" + strconv.FormatFloat(params.Contrast, 'f', -1, 64),
		"SATURATION=" + strconv.FormatFloat(params.Saturation, 'f', -1, 64),
		"SHARPNESS=" + strconv.FormatFloat(params.Sharpness, 'f', -1, 64),
		"EXPOSURE=" + params.Exposure,
		"AWB=" + params.AWB,
		"DENOISE=" + params.Denoise,
		"SHUTTER=" + strconv.FormatInt(int64(params.Shutter), 10),
		"METERING=" + params.Metering,
		"GAIN=" + strconv.FormatFloat(params.Gain, 'f', -1, 64),
		"EV=" + strconv.FormatFloat(params.EV, 'f', -1, 64),
		"ROI=" + params.ROI,
		"TUNING_FILE=" + params.TuningFile,
		"MODE=" + params.Mode,
		"FPS=" + strconv.FormatInt(int64(params.FPS), 10),
		"IDR_PERIOD=" + strconv.FormatInt(int64(params.IDRPeriod), 10),
		"BITRATE=" + strconv.FormatInt(int64(params.Bitrate), 10),
		"PROFILE=" + params.Profile,
		"LEVEL=" + params.Level,
	}

	exe, err := newEmbeddedExe(exeContent, env)
	if err != nil {
		pipe.close()
		return nil, err
	}

	waitDone := make(chan error)
	go func() {
		waitDone <- exe.cmd.Wait()
	}()

	readerDone := make(chan error)
	go func() {
		readerDone <- func() error {
			buf, err := pipe.read()
			if err != nil {
				return err
			}

			switch buf[0] {
			case 'e':
				return fmt.Errorf(string(buf[1:]))

			case 'r':
				return nil

			default:
				return fmt.Errorf("unexpected output from pipe (%c)", buf[0])
			}
		}()
	}()

	select {
	case <-waitDone:
		exe.close()
		pipe.close()
		<-readerDone
		return nil, fmt.Errorf("process exited unexpectedly")

	case err := <-readerDone:
		if err != nil {
			exe.close()
			<-waitDone
			pipe.close()
			return nil, err
		}
	}

	readerDone = make(chan error)
	go func() {
		readerDone <- func() error {
			for {
				buf, err := pipe.read()
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

				onData(dts, nalus)
			}
		}()
	}()

	return &RPICamera{
		onData:     onData,
		exe:        exe,
		pipe:       pipe,
		waitDone:   waitDone,
		readerDone: readerDone,
	}, nil
}

func (c *RPICamera) Close() {
	c.exe.close()
	<-c.waitDone
	c.pipe.close()
	<-c.readerDone
}
