package core

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/headers"
	"github.com/bluenviron/gortsplib/v4/pkg/sdp"
	rtspurl "github.com/bluenviron/gortsplib/v4/pkg/url"
	"github.com/datarhei/gosrt"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp"
	"github.com/bluenviron/mediamtx/internal/protocols/webrtc"
)

var runOnDemandSampleScript = `
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
	if os.Getenv("MTX_PATH") != "ondemand" ||
		os.Getenv("MTX_QUERY") != "param=value" ||
		os.Getenv("G1") != "on" {
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

	err = os.WriteFile("ON_DEMAND_FILE", []byte(""), 0644)
	if err != nil {
		panic(err)
	}
}
`

func TestPathRunOnDemand(t *testing.T) {
	onDemandFile := filepath.Join(os.TempDir(), "ondemand")
	onUnDemandFile := filepath.Join(os.TempDir(), "onundemand")

	srcFile := filepath.Join(os.TempDir(), "ondemand.go")
	err := os.WriteFile(srcFile,
		[]byte(strings.ReplaceAll(runOnDemandSampleScript, "ON_DEMAND_FILE", onDemandFile)), 0o644)
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
			defer os.Remove(onUnDemandFile)

			p1, ok := newInstance(fmt.Sprintf("rtmp: no\n"+
				"hls: no\n"+
				"webrtc: no\n"+
				"paths:\n"+
				"  '~^(on)demand$':\n"+
				"    runOnDemand: %s\n"+
				"    runOnDemandCloseAfter: 1s\n"+
				"    runOnUnDemand: touch %s\n", execFile, onUnDemandFile))
			require.Equal(t, true, ok)
			defer p1.Close()

			var control string

			func() {
				conn, err := net.Dial("tcp", "localhost:8554")
				require.NoError(t, err)
				defer conn.Close()
				br := bufio.NewReader(conn)

				if ca == "describe" || ca == "describe and setup" {
					u, err := rtspurl.Parse("rtsp://localhost:8554/ondemand?param=value")
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
					control = "rtsp://localhost:8554/ondemand?param=value/"
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
				_, err := os.Stat(onUnDemandFile)
				if err == nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}

			_, err := os.Stat(onDemandFile)
			require.NoError(t, err)
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
			"    runOnReady: sh -c 'echo \"$MTX_PATH $MTX_QUERY\" > %s'\n"+
			"    runOnNotReady: sh -c 'echo \"$MTX_PATH $MTX_QUERY\" > %s'\n",
			onReadyFile, onNotReadyFile))
		require.Equal(t, true, ok)
		defer p.Close()

		c := gortsplib.Client{}
		err := c.StartRecording(
			"rtsp://localhost:8554/test?query=value",
			&description.Session{Medias: []*description.Media{testMediaH264}})
		require.NoError(t, err)
		defer c.Close()

		time.Sleep(500 * time.Millisecond)
	}()

	byts, err := os.ReadFile(onReadyFile)
	require.NoError(t, err)
	require.Equal(t, "test query=value\n", string(byts))

	byts, err = os.ReadFile(onNotReadyFile)
	require.NoError(t, err)
	require.Equal(t, "test query=value\n", string(byts))
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
						"    runOnRead: sh -c 'echo \"$MTX_PATH $MTX_QUERY\" > %s'\n"+
						"    runOnUnread: sh -c 'echo \"$MTX_PATH $MTX_QUERY\" > %s'\n",
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

					u, err := rtspurl.Parse("rtsp://127.0.0.1:8554/test?query=value")
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
					u, err := url.Parse("rtmp://127.0.0.1:1935/test?query=value")
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

					u, err := url.Parse("http://localhost:8889/test/whep?query=value")
					require.NoError(t, err)

					c := &webrtc.WHIPClient{
						HTTPClient: hc,
						URL:        u,
					}

					_, err = c.Read(context.Background())
					require.NoError(t, err)
					defer checkClose(t, c.Close)
				}

				time.Sleep(500 * time.Millisecond)
			}()

			byts, err := os.ReadFile(onReadFile)
			require.NoError(t, err)
			if ca == "srt" {
				require.Equal(t, "test \n", string(byts))
			} else {
				require.Equal(t, "test query=value\n", string(byts))
			}

			byts, err = os.ReadFile(onUnreadFile)
			require.NoError(t, err)
			if ca == "srt" {
				require.Equal(t, "test \n", string(byts))
			} else {
				require.Equal(t, "test query=value\n", string(byts))
			}
		})
	}
}

func TestPathMaxReaders(t *testing.T) {
	p, ok := newInstance("paths:\n" +
		"  all_others:\n" +
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

func TestPathRecord(t *testing.T) {
	dir, err := os.MkdirTemp("", "rtsp-path-record")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	p, ok := newInstance("api: yes\n" +
		"record: yes\n" +
		"recordPath: " + filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f") + "\n" +
		"paths:\n" +
		"  all_others:\n" +
		"    record: yes\n")
	require.Equal(t, true, ok)
	defer p.Close()

	source := gortsplib.Client{}
	err = source.StartRecording(
		"rtsp://localhost:8554/mystream",
		&description.Session{Medias: []*description.Media{testMediaH264}})
	require.NoError(t, err)
	defer source.Close()

	for i := 0; i < 4; i++ {
		err := source.WritePacketRTP(testMediaH264, &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 1123 + uint16(i),
				Timestamp:      45343 + 90000*uint32(i),
				SSRC:           563423,
			},
			Payload: []byte{5},
		})
		require.NoError(t, err)
	}

	time.Sleep(500 * time.Millisecond)

	files, err := os.ReadDir(filepath.Join(dir, "mystream"))
	require.NoError(t, err)
	require.Equal(t, 1, len(files))

	hc := &http.Client{Transport: &http.Transport{}}

	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/paths/patch/all_others", map[string]interface{}{
		"record": false,
	}, nil)

	time.Sleep(500 * time.Millisecond)

	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/paths/patch/all_others", map[string]interface{}{
		"record": true,
	}, nil)

	time.Sleep(500 * time.Millisecond)

	for i := 4; i < 8; i++ {
		err := source.WritePacketRTP(testMediaH264, &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 1123 + uint16(i),
				Timestamp:      45343 + 90000*uint32(i),
				SSRC:           563423,
			},
			Payload: []byte{5},
		})
		require.NoError(t, err)
	}

	time.Sleep(500 * time.Millisecond)

	files, err = os.ReadDir(filepath.Join(dir, "mystream"))
	require.NoError(t, err)
	require.Equal(t, 2, len(files))
}
