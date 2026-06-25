//go:build (linux && arm) || (linux && arm64)

package rpicamera

import (
	"bytes"
	"os"
	"syscall"
	"unsafe"
)

type v4l2Capability struct {
	Driver       [16]byte
	Card         [32]byte
	BusInfo      [32]byte
	Version      uint32
	Capabilities uint32
	DeviceCaps   uint32
	Reserved     [3]uint32
}

const VIDIOC_QUERYCAP = 0x80685600

func supportsHardwareH264() bool {
	file, err := os.OpenFile("/dev/video11", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer file.Close()

	var caps v4l2Capability

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		file.Fd(),
		uintptr(VIDIOC_QUERYCAP),
		uintptr(unsafe.Pointer(&caps)),
	)
	if errno != 0 {
		return false
	}

	return bytes.HasPrefix(caps.Card[:], []byte("bcm2835-codec"))
}
