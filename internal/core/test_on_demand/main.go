// This is used for testing purposes.
package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
)

func main() {
	if os.Getenv("MTX_QUERY") != "param=value" {
		panic("unexpected MTX_QUERY")
	}
	if os.Getenv("G1") != "on" {
		panic("unexpected G1")
	}

	medi := &description.Media{
		Type: description.MediaTypeVideo,
		Formats: []format.Format{&format.H264{
			PayloadTyp: 96,
			SPS: []byte{
				0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
				0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
				0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
			},
			PPS:               []byte{0x01, 0x02, 0x03, 0x04},
			PacketizationMode: 1,
		}},
	}

	source := gortsplib.Client{}

	err := source.StartRecording(
		"rtsp://localhost:"+os.Getenv("RTSP_PORT")+"/"+os.Getenv("MTX_PATH"),
		&description.Session{Medias: []*description.Media{medi}})
	if err != nil {
		panic(err)
	}
	defer source.Close()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT)
	<-c

	err = os.WriteFile(os.Getenv("ON_DEMAND"), []byte(""), 0o644)
	if err != nil {
		panic(err)
	}
}
