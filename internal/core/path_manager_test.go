package core

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v3"
	"github.com/bluenviron/gortsplib/v3/pkg/base"
	"github.com/bluenviron/gortsplib/v3/pkg/headers"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/bluenviron/gortsplib/v3/pkg/sdp"
	"github.com/bluenviron/gortsplib/v3/pkg/url"
	"github.com/stretchr/testify/require"
)

func TestPathAutoDeletion(t *testing.T) {
	for _, ca := range []string{"describe", "setup"} {
		t.Run(ca, func(t *testing.T) {
			p, ok := newInstance("paths:\n" +
				"  all:\n")
			require.Equal(t, true, ok)
			defer p.Close()

			func() {
				conn, err := net.Dial("tcp", "localhost:8554")
				require.NoError(t, err)
				defer conn.Close()
				br := bufio.NewReader(conn)

				if ca == "describe" {
					u, err := url.Parse("rtsp://localhost:8554/mypath")
					require.NoError(t, err)

					byts, _ := base.Request{
						Method: base.Describe,
						URL:    u,
						Header: base.Header{
							"CSeq": base.HeaderValue{"1"},
						},
					}.Marshal()
					_, err = conn.Write(byts)
					require.NoError(t, err)

					var res base.Response
					err = res.Unmarshal(br)
					require.NoError(t, err)
					require.Equal(t, base.StatusNotFound, res.StatusCode)
				} else {
					u, err := url.Parse("rtsp://localhost:8554/mypath/trackID=0")
					require.NoError(t, err)

					byts, _ := base.Request{
						Method: base.Setup,
						URL:    u,
						Header: base.Header{
							"CSeq": base.HeaderValue{"1"},
							"Transport": headers.Transport{
								Mode: func() *headers.TransportMode {
									v := headers.TransportModePlay
									return &v
								}(),
								Delivery: func() *headers.TransportDelivery {
									v := headers.TransportDeliveryUnicast
									return &v
								}(),
								Protocol:    headers.TransportProtocolUDP,
								ClientPorts: &[2]int{35466, 35467},
							}.Marshal(),
						},
					}.Marshal()
					_, err = conn.Write(byts)
					require.NoError(t, err)

					var res base.Response
					err = res.Unmarshal(br)
					require.NoError(t, err)
					require.Equal(t, base.StatusNotFound, res.StatusCode)
				}
			}()

			data, err := p.pathManager.apiPathsList()
			require.NoError(t, err)

			require.Equal(t, 0, len(data.Items))
		})
	}
}

func TestPathRunOnDemand(t *testing.T) {
	doneFile := filepath.Join(os.TempDir(), "ondemand_done")

	srcFile := filepath.Join(os.TempDir(), "ondemand.go")
	err := os.WriteFile(srcFile, []byte(`
package main

import (
	"os"
	"os/signal"
	"syscall"
	"github.com/bluenviron/gortsplib/v3"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
)

func main() {
	if os.Getenv("G1") != "on" {
		panic("environment not set")
	}

	medi := &media.Media{
		Type: media.TypeVideo,
		Formats: []formats.Format{&formats.H264{
			PayloadTyp: 96,
			SPS: []byte{0x01, 0x02, 0x03, 0x04},
			PPS: []byte{0x01, 0x02, 0x03, 0x04},
			PacketizationMode: 1,
		}},
	}

	source := gortsplib.Client{}

	err := source.StartRecording(
		"rtsp://localhost:" + os.Getenv("RTSP_PORT") + "/" + os.Getenv("MTX_PATH"),
		media.Medias{medi})
	if err != nil {
		panic(err)
	}
	defer source.Close()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT)
	<-c

	err = os.WriteFile("`+doneFile+`", []byte(""), 0644)
	if err != nil {
		panic(err)
	}
}
`), 0o644)
	require.NoError(t, err)

	execFile := filepath.Join(os.TempDir(), "ondemand_cmd")
	cmd := exec.Command("go", "build", "-o", execFile, srcFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	require.NoError(t, err)
	defer os.Remove(execFile)

	os.Remove(srcFile)

	for _, ca := range []string{"describe", "setup", "describe and setup"} {
		t.Run(ca, func(t *testing.T) {
			defer os.Remove(doneFile)

			p1, ok := newInstance(fmt.Sprintf("rtmp: no\n"+
				"hls: no\n"+
				"webrtc: no\n"+
				"paths:\n"+
				"  '~^(on)demand$':\n"+
				"    runOnDemand: %s\n"+
				"    runOnDemandCloseAfter: 1s\n", execFile))
			require.Equal(t, true, ok)
			defer p1.Close()

			var control string

			func() {
				conn, err := net.Dial("tcp", "localhost:8554")
				require.NoError(t, err)
				defer conn.Close()
				br := bufio.NewReader(conn)

				if ca == "describe" || ca == "describe and setup" {
					u, err := url.Parse("rtsp://localhost:8554/ondemand")
					require.NoError(t, err)

					byts, _ := base.Request{
						Method: base.Describe,
						URL:    u,
						Header: base.Header{
							"CSeq": base.HeaderValue{"1"},
						},
					}.Marshal()
					_, err = conn.Write(byts)
					require.NoError(t, err)

					var res base.Response
					err = res.Unmarshal(br)
					require.NoError(t, err)
					require.Equal(t, base.StatusOK, res.StatusCode)

					var desc sdp.SessionDescription
					err = desc.Unmarshal(res.Body)
					require.NoError(t, err)
					control, _ = desc.MediaDescriptions[0].Attribute("control")
				} else {
					control = "rtsp://localhost:8554/ondemand/"
				}

				if ca == "setup" || ca == "describe and setup" {
					u, err := url.Parse(control)
					require.NoError(t, err)

					byts, _ := base.Request{
						Method: base.Setup,
						URL:    u,
						Header: base.Header{
							"CSeq": base.HeaderValue{"2"},
							"Transport": headers.Transport{
								Mode: func() *headers.TransportMode {
									v := headers.TransportModePlay
									return &v
								}(),
								Protocol:       headers.TransportProtocolTCP,
								InterleavedIDs: &[2]int{0, 1},
							}.Marshal(),
						},
					}.Marshal()
					_, err = conn.Write(byts)
					require.NoError(t, err)

					var res base.Response
					err = res.Unmarshal(br)
					require.NoError(t, err)
					require.Equal(t, base.StatusOK, res.StatusCode)
				}
			}()

			for {
				_, err := os.Stat(doneFile)
				if err == nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
		})
	}
}

func TestPathRunOnReady(t *testing.T) {
	doneFile := filepath.Join(os.TempDir(), "onready_done")
	defer os.Remove(doneFile)

	p, ok := newInstance(fmt.Sprintf("rtmp: no\n"+
		"hls: no\n"+
		"webrtc: no\n"+
		"paths:\n"+
		"  test:\n"+
		"    runOnReady: touch %s\n",
		doneFile))
	require.Equal(t, true, ok)
	defer p.Close()

	medi := testMediaH264

	c := gortsplib.Client{}

	err := c.StartRecording(
		"rtsp://localhost:8554/test",
		media.Medias{medi})
	require.NoError(t, err)
	defer c.Close()

	time.Sleep(1 * time.Second)

	_, err = os.Stat(doneFile)
	require.NoError(t, err)
}

func TestPathMaxReaders(t *testing.T) {
	p, ok := newInstance("paths:\n" +
		"  all:\n" +
		"    maxReaders: 1\n")
	require.Equal(t, true, ok)
	defer p.Close()

	source := gortsplib.Client{}
	err := source.StartRecording(
		"rtsp://localhost:8554/mystream",
		media.Medias{testMediaH264})
	require.NoError(t, err)
	defer source.Close()

	for i := 0; i < 2; i++ {
		reader := gortsplib.Client{}

		u, err := url.Parse("rtsp://127.0.0.1:8554/mystream")
		require.NoError(t, err)

		err = reader.Start(u.Scheme, u.Host)
		require.NoError(t, err)
		defer reader.Close()

		medias, baseURL, _, err := reader.Describe(u)
		require.NoError(t, err)

		err = reader.SetupAll(medias, baseURL)
		if i != 1 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}
