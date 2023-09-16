package core

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/headers"
	"github.com/bluenviron/gortsplib/v4/pkg/sdp"
	rtspurl "github.com/bluenviron/gortsplib/v4/pkg/url"
	"github.com/datarhei/gosrt"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/rtmp"
)

func TestPathRunOnDemand(t *testing.T) {
	onDemandFile := filepath.Join(os.TempDir(), "ondemand")

	srcFile := filepath.Join(os.TempDir(), "ondemand.go")
	err := os.WriteFile(srcFile, []byte(`
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
	if os.Getenv("G1") != "on" {
		panic("environment not set")
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
			PPS: []byte{0x01, 0x02, 0x03, 0x04},
			PacketizationMode: 1,
		}},
	}

	source := gortsplib.Client{}

	err := source.StartRecording(
		"rtsp://localhost:" + os.Getenv("RTSP_PORT") + "/" + os.Getenv("MTX_PATH"),
		&description.Session{Medias: []*description.Media{medi}})
	if err != nil {
		panic(err)
	}
	defer source.Close()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT)
	<-c

	err = os.WriteFile("`+onDemandFile+`", []byte(""), 0644)
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
			defer os.Remove(onDemandFile)

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
					u, err := rtspurl.Parse("rtsp://localhost:8554/ondemand")
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
					u, err := rtspurl.Parse(control)
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
				_, err := os.Stat(onDemandFile)
				if err == nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
		})
	}
}

func TestPathRunOnConnect(t *testing.T) {
	for _, ca := range []string{"rtsp", "rtmp", "srt"} {
		t.Run(ca, func(t *testing.T) {
			onConnectFile := filepath.Join(os.TempDir(), "onconnect")
			defer os.Remove(onConnectFile)

			onDisconnectFile := filepath.Join(os.TempDir(), "ondisconnect")
			defer os.Remove(onDisconnectFile)

			func() {
				p, ok := newInstance(fmt.Sprintf(
					"paths:\n"+
						"  test:\n"+
						"runOnConnect: touch %s\n"+
						"runOnDisconnect: touch %s\n",
					onConnectFile, onDisconnectFile))
				require.Equal(t, true, ok)
				defer p.Close()

				switch ca {
				case "rtsp":
					c := gortsplib.Client{}

					err := c.StartRecording(
						"rtsp://localhost:8554/test",
						&description.Session{Medias: []*description.Media{testMediaH264}})
					require.NoError(t, err)
					defer c.Close()

				case "rtmp":
					u, err := url.Parse("rtmp://127.0.0.1:1935/test")
					require.NoError(t, err)

					nconn, err := net.Dial("tcp", u.Host)
					require.NoError(t, err)
					defer nconn.Close()

					_, err = rtmp.NewClientConn(nconn, u, true)
					require.NoError(t, err)

				case "srt":
					conf := srt.DefaultConfig()
					address, err := conf.UnmarshalURL("srt://localhost:8890?streamid=publish:test")
					require.NoError(t, err)

					err = conf.Validate()
					require.NoError(t, err)

					c, err := srt.Dial("srt", address, conf)
					require.NoError(t, err)
					defer c.Close()
				}

				time.Sleep(500 * time.Millisecond)
			}()

			_, err := os.Stat(onConnectFile)
			require.NoError(t, err)

			_, err = os.Stat(onDisconnectFile)
			require.NoError(t, err)
		})
	}
}

func TestPathRunOnReady(t *testing.T) {
	onReadyFile := filepath.Join(os.TempDir(), "onready")
	defer os.Remove(onReadyFile)

	onNotReadyFile := filepath.Join(os.TempDir(), "onunready")
	defer os.Remove(onNotReadyFile)

	func() {
		p, ok := newInstance(fmt.Sprintf("rtmp: no\n"+
			"hls: no\n"+
			"webrtc: no\n"+
			"paths:\n"+
			"  test:\n"+
			"    runOnReady: touch %s\n"+
			"    runOnNotReady: touch %s\n",
			onReadyFile, onNotReadyFile))
		require.Equal(t, true, ok)
		defer p.Close()

		c := gortsplib.Client{}
		err := c.StartRecording(
			"rtsp://localhost:8554/test",
			&description.Session{Medias: []*description.Media{testMediaH264}})
		require.NoError(t, err)
		defer c.Close()

		time.Sleep(500 * time.Millisecond)
	}()

	_, err := os.Stat(onReadyFile)
	require.NoError(t, err)

	_, err = os.Stat(onNotReadyFile)
	require.NoError(t, err)
}

func TestPathRunOnRead(t *testing.T) {
	for _, ca := range []string{"rtsp", "rtmp", "srt", "webrtc"} {
		t.Run(ca, func(t *testing.T) {
			onReadFile := filepath.Join(os.TempDir(), "onread")
			defer os.Remove(onReadFile)

			onUnreadFile := filepath.Join(os.TempDir(), "onunread")
			defer os.Remove(onUnreadFile)

			func() {
				p, ok := newInstance(fmt.Sprintf(
					"paths:\n"+
						"  test:\n"+
						"    runOnRead: touch %s\n"+
						"    runOnUnread: touch %s\n",
					onReadFile, onUnreadFile))
				require.Equal(t, true, ok)
				defer p.Close()

				source := gortsplib.Client{}
				err := source.StartRecording(
					"rtsp://localhost:8554/test",
					&description.Session{Medias: []*description.Media{testMediaH264}})
				require.NoError(t, err)
				defer source.Close()

				switch ca {
				case "rtsp":
					reader := gortsplib.Client{}

					u, err := rtspurl.Parse("rtsp://127.0.0.1:8554/test")
					require.NoError(t, err)

					err = reader.Start(u.Scheme, u.Host)
					require.NoError(t, err)
					defer reader.Close()

					desc, _, err := reader.Describe(u)
					require.NoError(t, err)

					err = reader.SetupAll(desc.BaseURL, desc.Medias)
					require.NoError(t, err)

					_, err = reader.Play(nil)
					require.NoError(t, err)

				case "rtmp":
					u, err := url.Parse("rtmp://127.0.0.1:1935/test")
					require.NoError(t, err)

					nconn, err := net.Dial("tcp", u.Host)
					require.NoError(t, err)
					defer nconn.Close()

					conn, err := rtmp.NewClientConn(nconn, u, false)
					require.NoError(t, err)

					_, err = rtmp.NewReader(conn)
					require.NoError(t, err)

				case "srt":
					conf := srt.DefaultConfig()
					address, err := conf.UnmarshalURL("srt://localhost:8890?streamid=read:test")
					require.NoError(t, err)

					err = conf.Validate()
					require.NoError(t, err)

					reader, err := srt.Dial("srt", address, conf)
					require.NoError(t, err)
					defer reader.Close()

				case "webrtc":
					hc := &http.Client{Transport: &http.Transport{}}
					c := newWebRTCTestClient(t, hc, "http://localhost:8889/test/whep", false)
					defer c.close()
				}

				time.Sleep(500 * time.Millisecond)
			}()

			_, err := os.Stat(onReadFile)
			require.NoError(t, err)

			_, err = os.Stat(onUnreadFile)
			require.NoError(t, err)
		})
	}
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
		&description.Session{Medias: []*description.Media{
			testMediaH264,
			testMediaAAC,
		}})
	require.NoError(t, err)
	defer source.Close()

	for i := 0; i < 2; i++ {
		reader := gortsplib.Client{}

		u, err := rtspurl.Parse("rtsp://127.0.0.1:8554/mystream")
		require.NoError(t, err)

		err = reader.Start(u.Scheme, u.Host)
		require.NoError(t, err)
		defer reader.Close()

		desc, _, err := reader.Describe(u)
		require.NoError(t, err)

		err = reader.SetupAll(desc.BaseURL, desc.Medias)
		if i != 1 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}
