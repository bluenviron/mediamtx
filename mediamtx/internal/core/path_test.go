package core

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/headers"
	"github.com/bluenviron/gortsplib/v4/pkg/sdp"
	srt "github.com/datarhei/gosrt"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp"
	"github.com/bluenviron/mediamtx/internal/protocols/whip"
	"github.com/bluenviron/mediamtx/internal/test"
)

type testServer struct {
	onDescribe func(*gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error)
	onSetup    func(*gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error)
	onPlay     func(*gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error)
}

func (sh *testServer) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	return sh.onDescribe(ctx)
}

func (sh *testServer) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	return sh.onSetup(ctx)
}

func (sh *testServer) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	return sh.onPlay(ctx)
}

var _ defs.Path = &path{}

func TestPathRunOnDemand(t *testing.T) {
	onDemand := filepath.Join(os.TempDir(), "on_demand")
	onUnDemand := filepath.Join(os.TempDir(), "on_undemand")

	for _, ca := range []string{"describe", "setup", "describe and setup"} {
		t.Run(ca, func(t *testing.T) {
			defer os.Remove(onDemand)
			defer os.Remove(onUnDemand)

			p1, ok := newInstance(fmt.Sprintf("rtmp: no\n"+
				"hls: no\n"+
				"webrtc: no\n"+
				"paths:\n"+
				"  '~^(on)demand$':\n"+
				"    runOnDemand: sh -c \"ON_DEMAND=%s go run ./test_on_demand/main.go\"\n"+
				"    runOnDemandCloseAfter: 1s\n"+
				"    runOnUnDemand: touch %s\n", onDemand, onUnDemand))
			require.Equal(t, true, ok)
			defer p1.Close()

			var control string

			func() {
				conn, err := net.Dial("tcp", "localhost:8554")
				require.NoError(t, err)
				defer conn.Close()
				br := bufio.NewReader(conn)

				if ca == "describe" || ca == "describe and setup" {
					u, err := base.ParseURL("rtsp://localhost:8554/ondemand?param=value")
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
					control = "rtsp://localhost:8554/ondemand?param=value/" + control
				} else {
					control = "rtsp://localhost:8554/ondemand?param=value/"
				}

				if ca == "setup" || ca == "describe and setup" {
					u, err := base.ParseURL(control)
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
				_, err := os.Stat(onUnDemand)
				if err == nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}

			_, err := os.Stat(onDemand)
			require.NoError(t, err)
		})
	}
}

func TestPathRunOnConnect(t *testing.T) {
	serverCertFpath, err := test.CreateTempFile(test.TLSCertPub)
	require.NoError(t, err)
	defer os.Remove(serverCertFpath)

	serverKeyFpath, err := test.CreateTempFile(test.TLSCertKey)
	require.NoError(t, err)
	defer os.Remove(serverKeyFpath)

	for _, ca := range []string{"rtsp", "rtsps", "rtmp", "rtmps", "srt"} {
		t.Run(ca, func(t *testing.T) {
			onConnect := filepath.Join(os.TempDir(), "on_connect")
			defer os.Remove(onConnect)

			onDisconnect := filepath.Join(os.TempDir(), "on_disconnect")
			defer os.Remove(onDisconnect)

			connType := ""

			func() {
				p, ok := newInstance(fmt.Sprintf(
					"rtspEncryption: optional\n"+
						"rtspServerCert: "+serverCertFpath+"\n"+
						"rtspServerKey: "+serverKeyFpath+"\n"+
						"rtmpEncryption: optional\n"+
						"rtmpServerCert: "+serverCertFpath+"\n"+
						"rtmpServerKey: "+serverKeyFpath+"\n"+
						"paths:\n"+
						"  test:\n"+
						"runOnConnect: sh -c 'echo \"$MTX_CONN_TYPE $MTX_CONN_ID $RTSP_PORT\" > %s'\n"+
						"runOnDisconnect: sh -c 'echo \"$MTX_CONN_TYPE $MTX_CONN_ID $RTSP_PORT\" > %s'\n",
					onConnect, onDisconnect))
				require.Equal(t, true, ok)
				defer p.Close()

				switch ca {
				case "rtsp":
					connType = "rtspConn"

					c := gortsplib.Client{}

					err := c.StartRecording(
						"rtsp://localhost:8554/test",
						&description.Session{Medias: []*description.Media{test.UniqueMediaH264()}})
					require.NoError(t, err)
					defer c.Close()

				case "rtsps":
					connType = "rtspsConn"

					c := gortsplib.Client{TLSConfig: &tls.Config{InsecureSkipVerify: true}}

					err := c.StartRecording(
						"rtsps://localhost:8322/test",
						&description.Session{Medias: []*description.Media{test.UniqueMediaH264()}})
					require.NoError(t, err)
					defer c.Close()

				case "rtmp":
					connType = "rtmpConn"

					u, err := url.Parse("rtmp://127.0.0.1:1935/test")
					require.NoError(t, err)

					conn := &rtmp.Client{
						URL:     u,
						Publish: true,
					}
					err = conn.Initialize(context.Background())
					require.NoError(t, err)
					defer conn.Close()

				case "rtmps":
					connType = "rtmpsConn"

					u, err := url.Parse("rtmps://127.0.0.1:1936/test")
					require.NoError(t, err)

					conn := &rtmp.Client{
						URL:       u,
						Publish:   true,
						TLSConfig: &tls.Config{InsecureSkipVerify: true},
					}
					err = conn.Initialize(context.Background())
					require.NoError(t, err)
					defer conn.Close()

				case "srt":
					connType = "srtConn"

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

			byts, err := os.ReadFile(onConnect)
			require.NoError(t, err)
			fields := strings.Split(string(byts[:len(byts)-1]), " ")
			require.Equal(t, connType, fields[0])
			require.NotEmpty(t, fields[1])
			require.Equal(t, "8554", fields[2])

			byts, err = os.ReadFile(onDisconnect)
			require.NoError(t, err)
			fields = strings.Split(string(byts[:len(byts)-1]), " ")
			require.Equal(t, connType, fields[0])
			require.NotEmpty(t, fields[1])
			require.Equal(t, "8554", fields[2])
		})
	}
}

func TestPathRunOnReady(t *testing.T) {
	onReady := filepath.Join(os.TempDir(), "on_ready")
	defer os.Remove(onReady)

	onNotReady := filepath.Join(os.TempDir(), "on_unready")
	defer os.Remove(onNotReady)

	func() {
		p, ok := newInstance(fmt.Sprintf("rtmp: no\n"+
			"hls: no\n"+
			"webrtc: no\n"+
			"paths:\n"+
			"  ~te(st):\n"+
			"    runOnReady: sh -c 'echo \"$MTX_PATH $MTX_QUERY $MTX_SOURCE_TYPE $MTX_SOURCE_ID $RTSP_PORT $G1\" > %s'\n"+
			"    runOnNotReady: sh -c 'echo \"$MTX_PATH $MTX_QUERY $MTX_SOURCE_TYPE $MTX_SOURCE_ID $RTSP_PORT $G1\" > %s'\n",
			onReady, onNotReady))
		require.Equal(t, true, ok)
		defer p.Close()

		c := gortsplib.Client{}

		err := c.StartRecording(
			"rtsp://localhost:8554/test?query=value",
			&description.Session{Medias: []*description.Media{test.UniqueMediaH264()}})
		require.NoError(t, err)
		defer c.Close()

		time.Sleep(500 * time.Millisecond)
	}()

	byts, err := os.ReadFile(onReady)
	require.NoError(t, err)
	fields := strings.Split(string(byts[:len(byts)-1]), " ")
	require.Equal(t, "test", fields[0])
	require.Equal(t, "query=value", fields[1])
	require.Equal(t, "rtspSession", fields[2])
	require.NotEmpty(t, fields[3])
	require.Equal(t, "8554", fields[4])
	require.Equal(t, "st", fields[5])

	byts, err = os.ReadFile(onNotReady)
	require.NoError(t, err)
	fields = strings.Split(string(byts[:len(byts)-1]), " ")
	require.Equal(t, "test", fields[0])
	require.Equal(t, "query=value", fields[1])
	require.Equal(t, "rtspSession", fields[2])
	require.NotEmpty(t, fields[3])
	require.Equal(t, "8554", fields[4])
	require.Equal(t, "st", fields[5])
}

func TestPathRunOnRead(t *testing.T) {
	serverCertFpath, err := test.CreateTempFile(test.TLSCertPub)
	require.NoError(t, err)
	defer os.Remove(serverCertFpath)

	serverKeyFpath, err := test.CreateTempFile(test.TLSCertKey)
	require.NoError(t, err)
	defer os.Remove(serverKeyFpath)

	for _, ca := range []string{"rtsp", "rtsps", "rtmp", "rtmps", "srt", "webrtc"} {
		t.Run(ca, func(t *testing.T) {
			onRead := filepath.Join(os.TempDir(), "on_read")
			defer os.Remove(onRead)

			onUnread := filepath.Join(os.TempDir(), "on_unread")
			defer os.Remove(onUnread)

			func() {
				p, ok := newInstance(fmt.Sprintf(
					"rtspEncryption: optional\n"+
						"rtspServerCert: "+serverCertFpath+"\n"+
						"rtspServerKey: "+serverKeyFpath+"\n"+
						"rtmpEncryption: optional\n"+
						"rtmpServerCert: "+serverCertFpath+"\n"+
						"rtmpServerKey: "+serverKeyFpath+"\n"+
						"paths:\n"+
						"  ~te(st):\n"+
						"    runOnRead: sh -c 'echo \"$MTX_PATH $MTX_QUERY $MTX_READER_TYPE $MTX_READER_ID $RTSP_PORT $G1\" > %s'\n"+
						"    runOnUnread: sh -c 'echo \"$MTX_PATH $MTX_QUERY $MTX_READER_TYPE $MTX_READER_ID $RTSP_PORT $G1\" > %s'\n",
					onRead, onUnread))
				require.Equal(t, true, ok)
				defer p.Close()

				media0 := test.UniqueMediaH264()

				source := gortsplib.Client{}

				err := source.StartRecording(
					"rtsp://localhost:8554/test",
					&description.Session{Medias: []*description.Media{media0}})
				require.NoError(t, err)
				defer source.Close()

				writerDone := make(chan struct{})
				defer func() { <-writerDone }()

				writerTerminate := make(chan struct{})
				defer close(writerTerminate)

				go func() {
					defer close(writerDone)
					i := 0
					for {
						select {
						case <-time.After(100 * time.Millisecond):
						case <-writerTerminate:
							return
						}
						err2 := source.WritePacketRTP(media0, &rtp.Packet{
							Header: rtp.Header{
								Version:        2,
								Marker:         true,
								PayloadType:    96,
								SequenceNumber: uint16(123 + i),
								Timestamp:      uint32(45343 + i*90000),
								SSRC:           563423,
							},
							Payload: []byte{5},
						})
						require.NoError(t, err2)
						i++
					}
				}()

				switch ca {
				case "rtsp":
					u, err := base.ParseURL("rtsp://127.0.0.1:8554/test?query=value")
					require.NoError(t, err)

					reader := gortsplib.Client{
						Scheme: u.Scheme,
						Host:   u.Host,
					}

					err = reader.Start2()
					require.NoError(t, err)
					defer reader.Close()

					desc, _, err := reader.Describe(u)
					require.NoError(t, err)

					err = reader.SetupAll(desc.BaseURL, desc.Medias)
					require.NoError(t, err)

					_, err = reader.Play(nil)
					require.NoError(t, err)

				case "rtsps":
					u, err := base.ParseURL("rtsps://127.0.0.1:8322/test?query=value")
					require.NoError(t, err)

					reader := gortsplib.Client{
						Scheme:    u.Scheme,
						Host:      u.Host,
						TLSConfig: &tls.Config{InsecureSkipVerify: true},
					}

					err = reader.Start2()
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

					conn := &rtmp.Client{
						URL:     u,
						Publish: false,
					}
					err = conn.Initialize(context.Background())
					require.NoError(t, err)
					defer conn.Close()

					r := &rtmp.Reader{
						Conn: conn,
					}
					err = r.Initialize()
					require.NoError(t, err)

				case "rtmps":
					u, err := url.Parse("rtmps://127.0.0.1:1936/test?query=value")
					require.NoError(t, err)

					conn := &rtmp.Client{
						URL:       u,
						Publish:   false,
						TLSConfig: &tls.Config{InsecureSkipVerify: true},
					}
					err = conn.Initialize(context.Background())
					require.NoError(t, err)
					defer conn.Close()

					go func() {
						for i := uint16(0); i < 3; i++ {
							err2 := source.WritePacketRTP(media0, &rtp.Packet{
								Header: rtp.Header{
									Version:        2,
									Marker:         true,
									PayloadType:    96,
									SequenceNumber: 123 + i,
									Timestamp:      45343 + uint32(i)*2*90000,
									SSRC:           563423,
								},
								Payload: []byte{5},
							})
							require.NoError(t, err2)
						}
					}()

					r := &rtmp.Reader{
						Conn: conn,
					}
					err = r.Initialize()
					require.NoError(t, err)

				case "srt":
					conf := srt.DefaultConfig()
					address, err := conf.UnmarshalURL("srt://localhost:8890?streamid=read:test:query=value")
					require.NoError(t, err)

					err = conf.Validate()
					require.NoError(t, err)

					reader, err := srt.Dial("srt", address, conf)
					require.NoError(t, err)
					defer reader.Close()

				case "webrtc":
					tr := &http.Transport{}
					defer tr.CloseIdleConnections()
					hc := &http.Client{Transport: tr}

					u, err := url.Parse("http://localhost:8889/test/whep?query=value")
					require.NoError(t, err)

					c := &whip.Client{
						HTTPClient: hc,
						URL:        u,
						Log:        test.NilLogger,
					}

					err = c.Initialize(context.Background())
					require.NoError(t, err)
					defer checkClose(t, c.Close)
				}

				time.Sleep(500 * time.Millisecond)
			}()

			var readerType string

			switch ca {
			case "rtsp":
				readerType = "rtspSession"
			case "rtsps":
				readerType = "rtspsSession"
			case "rtmp":
				readerType = "rtmpConn"
			case "rtmps":
				readerType = "rtmpsConn"
			case "srt":
				readerType = "srtConn"
			case "webrtc":
				readerType = "webRTCSession"
			}

			byts, err := os.ReadFile(onRead)
			require.NoError(t, err)
			fields := strings.Split(string(byts[:len(byts)-1]), " ")
			require.Equal(t, "test", fields[0])
			require.Equal(t, "query=value", fields[1])
			require.Equal(t, readerType, fields[2])
			require.NotEmpty(t, fields[3])
			require.Equal(t, "8554", fields[4])
			require.Equal(t, "st", fields[5])

			byts, err = os.ReadFile(onUnread)
			require.NoError(t, err)
			fields = strings.Split(string(byts[:len(byts)-1]), " ")
			require.Equal(t, "test", fields[0])
			require.Equal(t, "query=value", fields[1])
			require.Equal(t, readerType, fields[2])
			require.NotEmpty(t, fields[3])
			require.Equal(t, "8554", fields[4])
			require.Equal(t, "st", fields[5])
		})
	}
}

func TestPathRunOnRecordSegment(t *testing.T) {
	onRecordSegmentCreate := filepath.Join(os.TempDir(), "on_record_segment_create")
	defer os.Remove(onRecordSegmentCreate)

	onRecordSegmentComplete := filepath.Join(os.TempDir(), "on_record_segment_complete")
	defer os.Remove(onRecordSegmentComplete)

	recordDir, err := os.MkdirTemp("", "rtsp-path-record")
	require.NoError(t, err)
	defer os.RemoveAll(recordDir)

	func() {
		p, ok := newInstance(fmt.Sprintf("record: yes\n"+
			"recordPath: %s\n"+
			"paths:\n"+
			"  test:\n"+
			"    runOnRecordSegmentCreate: sh -c 'echo \"$MTX_SEGMENT_PATH $RTSP_PORT\" > %s'\n"+
			"    runOnRecordSegmentComplete: sh -c 'echo \"$MTX_SEGMENT_PATH $MTX_SEGMENT_DURATION $RTSP_PORT\" > %s'\n",
			filepath.Join(recordDir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
			onRecordSegmentCreate,
			onRecordSegmentComplete,
		))
		require.Equal(t, true, ok)
		defer p.Close()

		media0 := test.UniqueMediaH264()

		source := gortsplib.Client{}

		err = source.StartRecording(
			"rtsp://localhost:8554/test",
			&description.Session{Medias: []*description.Media{media0}})
		require.NoError(t, err)
		defer source.Close()

		for i := 0; i < 4; i++ {
			err = source.WritePacketRTP(media0, &rtp.Packet{
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
	}()

	byts, err := os.ReadFile(onRecordSegmentCreate)
	require.NoError(t, err)
	fields := strings.Split(string(byts[:len(byts)-1]), " ")
	require.True(t, strings.HasPrefix(fields[0], recordDir))
	require.Equal(t, "8554", fields[1])

	byts, err = os.ReadFile(onRecordSegmentComplete)
	require.NoError(t, err)
	fields = strings.Split(string(byts[:len(byts)-1]), " ")
	require.True(t, strings.HasPrefix(fields[0], recordDir))
	require.Equal(t, "3", fields[1])
	require.Equal(t, "8554", fields[2])
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
			test.UniqueMediaH264(),
			test.UniqueMediaMPEG4Audio(),
		}})
	require.NoError(t, err)
	defer source.Close()

	for i := 0; i < 2; i++ {
		u, err := base.ParseURL("rtsp://127.0.0.1:8554/mystream")
		require.NoError(t, err)

		reader := gortsplib.Client{
			Scheme: u.Scheme,
			Host:   u.Host,
		}

		err = reader.Start2()
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

	media0 := test.UniqueMediaH264()

	source := gortsplib.Client{}

	err = source.StartRecording(
		"rtsp://localhost:8554/mystream",
		&description.Session{Medias: []*description.Media{media0}})
	require.NoError(t, err)
	defer source.Close()

	for i := 0; i < 4; i++ {
		err = source.WritePacketRTP(media0, &rtp.Packet{
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

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/paths/patch/all_others", map[string]interface{}{
		"record": false,
	}, nil)

	time.Sleep(500 * time.Millisecond)

	httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/paths/patch/all_others", map[string]interface{}{
		"record": true,
	}, nil)

	time.Sleep(500 * time.Millisecond)

	for i := 4; i < 8; i++ {
		err = source.WritePacketRTP(media0, &rtp.Packet{
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

func TestPathFallback(t *testing.T) {
	for _, ca := range []string{
		"absolute",
		"relative",
		"source",
	} {
		t.Run(ca, func(t *testing.T) {
			var conf string

			switch ca {
			case "absolute":
				conf = "paths:\n" +
					"  path1:\n" +
					"    fallback: rtsp://localhost:8554/path2\n" +
					"  path2:\n"

			case "relative":
				conf = "paths:\n" +
					"  path1:\n" +
					"    fallback: /path2\n" +
					"  path2:\n"

			case "source":
				conf = "paths:\n" +
					"  path1:\n" +
					"    fallback: /path2\n" +
					"    source: rtsp://localhost:3333/nonexistent\n" +
					"  path2:\n"
			}

			p1, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p1.Close()

			source := gortsplib.Client{}
			err := source.StartRecording("rtsp://localhost:8554/path2",
				&description.Session{Medias: []*description.Media{test.UniqueMediaH264()}})
			require.NoError(t, err)
			defer source.Close()

			u, err := base.ParseURL("rtsp://localhost:8554/path1")
			require.NoError(t, err)

			dest := gortsplib.Client{
				Scheme: u.Scheme,
				Host:   u.Host,
			}

			err = dest.Start2()
			require.NoError(t, err)
			defer dest.Close()

			desc, _, err := dest.Describe(u)
			require.NoError(t, err)
			require.Equal(t, 1, len(desc.Medias))
		})
	}
}

func TestPathResolveSource(t *testing.T) {
	var stream *gortsplib.ServerStream

	s := gortsplib.Server{
		Handler: &testServer{
			onDescribe: func(ctx *gortsplib.ServerHandlerOnDescribeCtx,
			) (*base.Response, *gortsplib.ServerStream, error) {
				require.Equal(t, "key=val", ctx.Query)
				require.Equal(t, "/a", ctx.Path)
				return &base.Response{
					StatusCode: base.StatusOK,
				}, stream, nil
			},
			onSetup: func(_ *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, stream, nil
			},
			onPlay: func(_ *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil
			},
		},
		RTSPAddress: "127.0.0.1:8555",
	}

	err := s.Start()
	require.NoError(t, err)
	defer s.Close()

	stream = &gortsplib.ServerStream{
		Server: &s,
		Desc:   &description.Session{Medias: []*description.Media{test.MediaH264}},
	}
	err = stream.Initialize()
	require.NoError(t, err)
	defer stream.Close()

	p, ok := newInstance(
		"paths:\n" +
			"  '~^test_(.+)$':\n" +
			"    source: rtsp://127.0.0.1:8555/$G1?$MTX_QUERY\n" +
			"    sourceOnDemand: yes\n" +
			"  'all':\n")
	require.Equal(t, true, ok)
	defer p.Close()

	u, err := base.ParseURL("rtsp://127.0.0.1:8554/test_a?key=val")
	require.NoError(t, err)

	reader := gortsplib.Client{
		Scheme: u.Scheme,
		Host:   u.Host,
	}

	err = reader.Start2()
	require.NoError(t, err)
	defer reader.Close()

	_, _, err = reader.Describe(u)
	require.NoError(t, err)
}

func TestPathOverridePublisher(t *testing.T) {
	for _, ca := range []string{
		"enabled",
		"disabled",
	} {
		t.Run(ca, func(t *testing.T) {
			conf := "rtmp: no\n" +
				"paths:\n" +
				"  all_others:\n"

			if ca == "disabled" {
				conf += "    overridePublisher: no\n"
			}

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.Close()

			medi := test.UniqueMediaH264()

			s1 := gortsplib.Client{}

			err := s1.StartRecording("rtsp://localhost:8554/teststream",
				&description.Session{Medias: []*description.Media{medi}})
			require.NoError(t, err)
			defer s1.Close()

			s2 := gortsplib.Client{}

			err = s2.StartRecording("rtsp://localhost:8554/teststream",
				&description.Session{Medias: []*description.Media{medi}})
			if ca == "enabled" {
				require.NoError(t, err)
				defer s2.Close()
			} else {
				require.Error(t, err)
			}

			frameRecv := make(chan struct{})

			u, err := base.ParseURL("rtsp://localhost:8554/teststream")
			require.NoError(t, err)

			c := gortsplib.Client{
				Scheme: u.Scheme,
				Host:   u.Host,
			}

			err = c.Start2()
			require.NoError(t, err)
			defer c.Close()

			desc, _, err := c.Describe(u)
			require.NoError(t, err)

			err = c.SetupAll(desc.BaseURL, desc.Medias)
			require.NoError(t, err)

			c.OnPacketRTP(desc.Medias[0], desc.Medias[0].Formats[0], func(pkt *rtp.Packet) {
				if ca == "enabled" {
					require.Equal(t, []byte{5, 15, 16, 17, 18}, pkt.Payload)
				} else {
					require.Equal(t, []byte{5, 11, 12, 13, 14}, pkt.Payload)
				}
				close(frameRecv)
			})

			_, err = c.Play(nil)
			require.NoError(t, err)

			if ca == "enabled" {
				err = s1.Wait()
				require.EqualError(t, err, "EOF")

				err = s2.WritePacketRTP(medi, &rtp.Packet{
					Header: rtp.Header{
						Version:        0x02,
						PayloadType:    96,
						SequenceNumber: 57899,
						Timestamp:      345234345,
						SSRC:           978651231,
						Marker:         true,
					},
					Payload: []byte{5, 15, 16, 17, 18},
				})
				require.NoError(t, err)
			} else {
				err = s1.WritePacketRTP(medi, &rtp.Packet{
					Header: rtp.Header{
						Version:        0x02,
						PayloadType:    96,
						SequenceNumber: 57899,
						Timestamp:      345234345,
						SSRC:           978651231,
						Marker:         true,
					},
					Payload: []byte{5, 11, 12, 13, 14},
				})
				require.NoError(t, err)
			}

			<-frameRecv
		})
	}
}
