package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/headers"
	"github.com/stretchr/testify/require"
)

func TestRTSPPublishRead(t *testing.T) {
	for _, ca := range []struct {
		encrypted      bool
		publisherSoft  string
		publisherProto string
		readerSoft     string
		readerProto    string
	}{
		{false, "ffmpeg", "udp", "ffmpeg", "udp"},
		{false, "ffmpeg", "udp", "ffmpeg", "tcp"},
		{false, "ffmpeg", "udp", "gstreamer", "udp"},
		{false, "ffmpeg", "udp", "gstreamer", "tcp"},
		{false, "ffmpeg", "udp", "vlc", "udp"},
		{false, "ffmpeg", "udp", "vlc", "tcp"},

		{false, "ffmpeg", "tcp", "ffmpeg", "udp"},
		{false, "gstreamer", "udp", "ffmpeg", "udp"},
		{false, "gstreamer", "tcp", "ffmpeg", "udp"},

		{true, "ffmpeg", "tcp", "ffmpeg", "tcp"},
		{true, "ffmpeg", "tcp", "gstreamer", "tcp"},
		{true, "gstreamer", "tcp", "ffmpeg", "tcp"},
	} {
		encryptedStr := func() string {
			if ca.encrypted {
				return "encrypted"
			}
			return "plain"
		}()

		t.Run(encryptedStr+"_"+ca.publisherSoft+"_"+ca.publisherProto+"_"+
			ca.readerSoft+"_"+ca.readerProto, func(t *testing.T) {
			var proto string
			var port string
			if !ca.encrypted {
				proto = "rtsp"
				port = "8554"

				p, ok := testProgram("rtmpDisable: yes\n" +
					"readTimeout: 20s\n")
				require.Equal(t, true, ok)
				defer p.close()

			} else {
				proto = "rtsps"
				port = "8555"

				serverCertFpath, err := writeTempFile(serverCert)
				require.NoError(t, err)
				defer os.Remove(serverCertFpath)

				serverKeyFpath, err := writeTempFile(serverKey)
				require.NoError(t, err)
				defer os.Remove(serverKeyFpath)

				p, ok := testProgram("rtmpDisable: yes\n" +
					"readTimeout: 20s\n" +
					"protocols: [tcp]\n" +
					"encryption: yes\n" +
					"serverCert: " + serverCertFpath + "\n" +
					"serverKey: " + serverKeyFpath + "\n")
				require.Equal(t, true, ok)
				defer p.close()
			}

			switch ca.publisherSoft {
			case "ffmpeg":
				cnt1, err := newContainer("ffmpeg", "source", []string{
					"-re",
					"-stream_loop", "-1",
					"-i", "emptyvideo.ts",
					"-c", "copy",
					"-f", "rtsp",
					"-rtsp_transport", ca.publisherProto,
					proto + "://" + ownDockerIP + ":" + port + "/teststream",
				})
				require.NoError(t, err)
				defer cnt1.close()

				time.Sleep(1 * time.Second)

			case "gstreamer":
				cnt1, err := newContainer("gstreamer", "source", []string{
					"filesrc location=emptyvideo.ts ! tsdemux ! video/x-h264 ! rtspclientsink " +
						"location=" + proto + "://" + ownDockerIP + ":" + port + "/teststream " +
						"protocols=" + ca.publisherProto + " tls-validation-flags=0 latency=0 timeout=0 rtx-time=0",
				})
				require.NoError(t, err)
				defer cnt1.close()

				time.Sleep(1 * time.Second)
			}

			time.Sleep(1 * time.Second)

			switch ca.readerSoft {
			case "ffmpeg":
				cnt2, err := newContainer("ffmpeg", "dest", []string{
					"-rtsp_transport", ca.readerProto,
					"-i", proto + "://" + ownDockerIP + ":" + port + "/teststream",
					"-vframes", "1",
					"-f", "image2",
					"-y", "/dev/null",
				})
				require.NoError(t, err)
				defer cnt2.close()
				require.Equal(t, 0, cnt2.wait())

			case "gstreamer":
				cnt2, err := newContainer("gstreamer", "read", []string{
					"rtspsrc location=" + proto + "://" + ownDockerIP + ":" + port + "/teststream protocols=tcp tls-validation-flags=0 latency=0 " +
						"! application/x-rtp,media=video ! decodebin ! exitafterframe ! fakesink",
				})
				require.NoError(t, err)
				defer cnt2.close()
				require.Equal(t, 0, cnt2.wait())

			case "vlc":
				args := []string{}
				if ca.readerProto == "tcp" {
					args = append(args, "--rtsp-tcp")
				}
				args = append(args, proto+"://"+ownDockerIP+":"+port+"/teststream")
				cnt2, err := newContainer("vlc", "dest", args)
				require.NoError(t, err)
				defer cnt2.close()
				require.Equal(t, 0, cnt2.wait())
			}
		})
	}
}

func TestRTSPAuth(t *testing.T) {
	t.Run("publish", func(t *testing.T) {
		p, ok := testProgram("rtmpDisable: yes\n" +
			"paths:\n" +
			"  all:\n" +
			"    publishUser: testuser\n" +
			"    publishPass: test!$()*+.;<=>[]^_-{}\n" +
			"    publishIps: [172.17.0.0/16]\n")
		require.Equal(t, true, ok)
		defer p.close()

		cnt1, err := newContainer("ffmpeg", "source", []string{
			"-re",
			"-stream_loop", "-1",
			"-i", "emptyvideo.ts",
			"-c", "copy",
			"-f", "rtsp",
			"-rtsp_transport", "udp",
			"rtsp://testuser:test!$()*+.;<=>[]^_-{}@" + ownDockerIP + ":8554/test/stream",
		})
		require.NoError(t, err)
		defer cnt1.close()

		time.Sleep(1 * time.Second)

		cnt2, err := newContainer("ffmpeg", "dest", []string{
			"-rtsp_transport", "udp",
			"-i", "rtsp://" + ownDockerIP + ":8554/test/stream",
			"-vframes", "1",
			"-f", "image2",
			"-y", "/dev/null",
		})
		require.NoError(t, err)
		defer cnt2.close()
		require.Equal(t, 0, cnt2.wait())
	})

	for _, soft := range []string{
		"ffmpeg",
		"vlc",
	} {
		t.Run("read_"+soft, func(t *testing.T) {
			p, ok := testProgram("rtmpDisable: yes\n" +
				"paths:\n" +
				"  all:\n" +
				"    readUser: testuser\n" +
				"    readPass: test!$()*+.;<=>[]^_-{}\n" +
				"    readIps: [172.17.0.0/16]\n")
			require.Equal(t, true, ok)
			defer p.close()

			cnt1, err := newContainer("ffmpeg", "source", []string{
				"-re",
				"-stream_loop", "-1",
				"-i", "emptyvideo.ts",
				"-c", "copy",
				"-f", "rtsp",
				"-rtsp_transport", "udp",
				"rtsp://" + ownDockerIP + ":8554/test/stream",
			})
			require.NoError(t, err)
			defer cnt1.close()

			time.Sleep(1 * time.Second)

			if soft == "ffmpeg" {
				cnt2, err := newContainer("ffmpeg", "dest", []string{
					"-rtsp_transport", "udp",
					"-i", "rtsp://testuser:test!$()*+.;<=>[]^_-{}@" + ownDockerIP + ":8554/test/stream",
					"-vframes", "1",
					"-f", "image2",
					"-y", "/dev/null",
				})
				require.NoError(t, err)
				defer cnt2.close()
				require.Equal(t, 0, cnt2.wait())

			} else {
				cnt2, err := newContainer("vlc", "dest", []string{
					"rtsp://testuser:test!$()*+.;<=>[]^_-{}@" + ownDockerIP + ":8554/test/stream",
				})
				require.NoError(t, err)
				defer cnt2.close()
				require.Equal(t, 0, cnt2.wait())
			}
		})
	}

	t.Run("hashed", func(t *testing.T) {
		p, ok := testProgram("rtmpDisable: yes\n" +
			"paths:\n" +
			"  all:\n" +
			"    readUser: sha256:rl3rgi4NcZkpAEcacZnQ2VuOfJ0FxAqCRaKB/SwdZoQ=\n" +
			"    readPass: sha256:E9JJ8stBJ7QM+nV4ZoUCeHk/gU3tPFh/5YieiJp6n2w=\n")
		require.Equal(t, true, ok)
		defer p.close()

		cnt1, err := newContainer("ffmpeg", "source", []string{
			"-re",
			"-stream_loop", "-1",
			"-i", "emptyvideo.ts",
			"-c", "copy",
			"-f", "rtsp",
			"-rtsp_transport", "udp",
			"rtsp://" + ownDockerIP + ":8554/test/stream",
		})
		require.NoError(t, err)
		defer cnt1.close()

		cnt2, err := newContainer("ffmpeg", "dest", []string{
			"-rtsp_transport", "udp",
			"-i", "rtsp://testuser:testpass@" + ownDockerIP + ":8554/test/stream",
			"-vframes", "1",
			"-f", "image2",
			"-y", "/dev/null",
		})
		require.NoError(t, err)
		defer cnt2.close()
		require.Equal(t, 0, cnt2.wait())
	})
}

func TestRTSPAuthFail(t *testing.T) {
	for _, ca := range []struct {
		name string
		user string
		pass string
	}{
		{
			"publish_wronguser",
			"test1user",
			"testpass",
		},
		{
			"publish_wrongpass",
			"testuser",
			"test1pass",
		},
		{
			"publish_wrongboth",
			"test1user",
			"test1pass",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			p, ok := testProgram(
				"rtmpDisable: yes\n" +
					"paths:\n" +
					"  all:\n" +
					"    publishUser: testuser\n" +
					"    publishPass: testpass\n")
			require.Equal(t, true, ok)
			defer p.close()

			cnt1, err := newContainer("ffmpeg", "source", []string{
				"-re",
				"-stream_loop", "-1",
				"-i", "emptyvideo.ts",
				"-c", "copy",
				"-f", "rtsp",
				"-rtsp_transport", "udp",
				"rtsp://" + ca.user + ":" + ca.pass + "@" + ownDockerIP + ":8554/test/stream",
			})
			require.NoError(t, err)
			defer cnt1.close()

			time.Sleep(1 * time.Second)

			cnt2, err := newContainer("ffmpeg", "dest", []string{
				"-rtsp_transport", "udp",
				"-i", "rtsp://" + ownDockerIP + ":8554/test/stream",
				"-vframes", "1",
				"-f", "image2",
				"-y", "/dev/null",
			})
			require.NoError(t, err)
			defer cnt2.close()
			require.Equal(t, 1, cnt2.wait())
		})
	}

	for _, ca := range []struct {
		name string
		user string
		pass string
	}{
		{
			"read_wronguser",
			"test1user",
			"testpass",
		},
		{
			"read_wrongpass",
			"testuser",
			"test1pass",
		},
		{
			"read_wrongboth",
			"test1user",
			"test1pass",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			p, ok := testProgram(
				"rtmpDisable: yes\n" +
					"paths:\n" +
					"  all:\n" +
					"    readUser: testuser\n" +
					"    readPass: testpass\n")
			require.Equal(t, true, ok)
			defer p.close()

			cnt1, err := newContainer("ffmpeg", "source", []string{
				"-re",
				"-stream_loop", "-1",
				"-i", "emptyvideo.ts",
				"-c", "copy",
				"-f", "rtsp",
				"-rtsp_transport", "udp",
				"rtsp://" + ownDockerIP + ":8554/test/stream",
			})
			require.NoError(t, err)
			defer cnt1.close()

			time.Sleep(1 * time.Second)

			cnt2, err := newContainer("ffmpeg", "dest", []string{
				"-rtsp_transport", "udp",
				"-i", "rtsp://" + ca.user + ":" + ca.pass + "@" + ownDockerIP + ":8554/test/stream",
				"-vframes", "1",
				"-f", "image2",
				"-y", "/dev/null",
			})
			require.NoError(t, err)
			defer cnt2.close()
			require.Equal(t, 1, cnt2.wait())
		})
	}
}

func TestRTSPAuthIpFail(t *testing.T) {
	p, ok := testProgram("rtmpDisable: yes\n" +
		"paths:\n" +
		"  all:\n" +
		"    publishIps: [127.0.0.1/32]\n")
	require.Equal(t, true, ok)
	defer p.close()

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.ts",
		"-c", "copy",
		"-f", "rtsp",
		"-rtsp_transport", "udp",
		"rtsp://" + ownDockerIP + ":8554/test/stream",
	})
	require.NoError(t, err)
	defer cnt1.close()
	require.NotEqual(t, 0, cnt1.wait())
}

func TestRTSPAutomaticProtocol(t *testing.T) {
	for _, source := range []string{
		"ffmpeg",
	} {
		t.Run(source, func(t *testing.T) {
			p, ok := testProgram("rtmpDisable: yes\n" +
				"protocols: [tcp]\n")
			require.Equal(t, true, ok)
			defer p.close()

			switch source {
			case "ffmpeg":
				cnt1, err := newContainer("ffmpeg", "source", []string{
					"-re",
					"-stream_loop", "-1",
					"-i", "emptyvideo.ts",
					"-c", "copy",
					"-f", "rtsp",
					"rtsp://" + ownDockerIP + ":8554/teststream",
				})
				require.NoError(t, err)
				defer cnt1.close()
			}

			cnt2, err := newContainer("ffmpeg", "dest", []string{
				"-i", "rtsp://" + ownDockerIP + ":8554/teststream",
				"-vframes", "1",
				"-f", "image2",
				"-y", "/dev/null",
			})
			require.NoError(t, err)
			defer cnt2.close()
			require.Equal(t, 0, cnt2.wait())
		})
	}
}

func TestRTSPPublisherOverride(t *testing.T) {
	p, ok := testProgram("rtmpDisable: yes\n")
	require.Equal(t, true, ok)
	defer p.close()

	source1, err := newContainer("ffmpeg", "source1", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.ts",
		"-c", "copy",
		"-f", "rtsp",
		"rtsp://" + ownDockerIP + ":8554/teststream",
	})
	require.NoError(t, err)
	defer source1.close()

	time.Sleep(1 * time.Second)

	source2, err := newContainer("ffmpeg", "source2", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.ts",
		"-c", "copy",
		"-f", "rtsp",
		"rtsp://" + ownDockerIP + ":8554/teststream",
	})
	require.NoError(t, err)
	defer source2.close()

	time.Sleep(1 * time.Second)

	dest, err := newContainer("ffmpeg", "dest", []string{
		"-i", "rtsp://" + ownDockerIP + ":8554/teststream",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer dest.close()
	require.Equal(t, 0, dest.wait())
}

func TestRTSPPath(t *testing.T) {
	for _, ca := range []struct {
		name string
		path string
	}{
		{
			"with slash",
			"test/stream",
		},
		{
			"with query",
			"test?param1=val&param2=val",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			p, ok := testProgram("rtmpDisable: yes\n")
			require.Equal(t, true, ok)
			defer p.close()

			cnt1, err := newContainer("ffmpeg", "source", []string{
				"-re",
				"-stream_loop", "-1",
				"-i", "emptyvideo.ts",
				"-c", "copy",
				"-f", "rtsp",
				"-rtsp_transport", "udp",
				"rtsp://" + ownDockerIP + ":8554/" + ca.path,
			})
			require.NoError(t, err)
			defer cnt1.close()

			cnt2, err := newContainer("ffmpeg", "dest", []string{
				"-rtsp_transport", "udp",
				"-i", "rtsp://" + ownDockerIP + ":8554/" + ca.path,
				"-vframes", "1",
				"-f", "image2",
				"-y", "/dev/null",
			})
			require.NoError(t, err)
			defer cnt2.close()
			require.Equal(t, 0, cnt2.wait())
		})
	}
}

func TestRTSPNonCompliantFrameSize(t *testing.T) {
	t.Run("publish", func(t *testing.T) {
		p, ok := testProgram("rtmpDisable: yes\n" +
			"readBufferSize: 4500\n")
		require.Equal(t, true, ok)
		defer p.close()

		track, err := gortsplib.NewTrackH264(96, []byte("123456"), []byte("123456"))
		require.NoError(t, err)

		conf := gortsplib.ClientConf{
			StreamProtocol: func() *gortsplib.StreamProtocol {
				v := gortsplib.StreamProtocolTCP
				return &v
			}(),
		}

		source, err := conf.DialPublish("rtsp://"+ownDockerIP+":8554/teststream",
			gortsplib.Tracks{track})
		require.NoError(t, err)
		defer source.Close()

		buf := bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04, 0x05}, 4096/5)
		err = source.WriteFrame(track.ID, gortsplib.StreamTypeRTP, buf)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		err = source.WriteFrame(track.ID, gortsplib.StreamTypeRTP, buf)
		require.NoError(t, err)
	})

	t.Run("proxy", func(t *testing.T) {
		p1, ok := testProgram("rtmpDisable: yes\n" +
			"protocols: [tcp]\n" +
			"readBufferSize: 4500\n")
		require.Equal(t, true, ok)
		defer p1.close()

		track, err := gortsplib.NewTrackH264(96, []byte("123456"), []byte("123456"))
		require.NoError(t, err)

		conf := gortsplib.ClientConf{
			StreamProtocol: func() *gortsplib.StreamProtocol {
				v := gortsplib.StreamProtocolTCP
				return &v
			}(),
			ReadBufferSize: 4500,
		}

		source, err := conf.DialPublish("rtsp://"+ownDockerIP+":8554/teststream",
			gortsplib.Tracks{track})
		require.NoError(t, err)
		defer source.Close()

		p2, ok := testProgram("rtmpDisable: yes\n" +
			"protocols: [tcp]\n" +
			"readBufferSize: 4500\n" +
			"rtspPort: 8555\n" +
			"paths:\n" +
			"  teststream:\n" +
			"    source: rtsp://" + ownDockerIP + ":8554/teststream\n")
		require.Equal(t, true, ok)
		defer p2.close()

		time.Sleep(100 * time.Millisecond)

		dest, err := conf.DialRead("rtsp://" + ownDockerIP + ":8555/teststream")
		require.NoError(t, err)
		defer dest.Close()

		done := make(chan struct{})
		cerr := dest.ReadFrames(func(trackID int, typ gortsplib.StreamType, buf []byte) {
			if typ == gortsplib.StreamTypeRTP {
				close(done)
			}
		})

		buf := bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04, 0x05}, 4096/5)
		err = source.WriteFrame(track.ID, gortsplib.StreamTypeRTP, buf)
		require.NoError(t, err)

		select {
		case err := <-cerr:
			t.Error(err)
		case <-done:
		}
	})
}

func TestRTSPRedirect(t *testing.T) {
	p1, ok := testProgram("rtmpDisable: yes\n" +
		"paths:\n" +
		"  path1:\n" +
		"    source: redirect\n" +
		"    sourceRedirect: rtsp://" + ownDockerIP + ":8554/path2\n" +
		"  path2:\n")
	require.Equal(t, true, ok)
	defer p1.close()

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.ts",
		"-c", "copy",
		"-f", "rtsp",
		"-rtsp_transport", "udp",
		"rtsp://" + ownDockerIP + ":8554/path2",
	})
	require.NoError(t, err)
	defer cnt1.close()

	time.Sleep(1 * time.Second)

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-rtsp_transport", "udp",
		"-i", "rtsp://" + ownDockerIP + ":8554/path1",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()
	require.Equal(t, 0, cnt2.wait())
}

func TestRTSPFallback(t *testing.T) {
	for _, ca := range []string{
		"absolute",
		"relative",
	} {
		t.Run(ca, func(t *testing.T) {
			val := func() string {
				if ca == "absolute" {
					return "rtsp://" + ownDockerIP + ":8554/path2"
				}
				return "/path2"
			}()

			p1, ok := testProgram("rtmpDisable: yes\n" +
				"paths:\n" +
				"  path1:\n" +
				"    fallback: " + val + "\n" +
				"  path2:\n")
			require.Equal(t, true, ok)
			defer p1.close()

			cnt1, err := newContainer("ffmpeg", "source", []string{
				"-re",
				"-stream_loop", "-1",
				"-i", "emptyvideo.ts",
				"-c", "copy",
				"-f", "rtsp",
				"-rtsp_transport", "udp",
				"rtsp://" + ownDockerIP + ":8554/path2",
			})
			require.NoError(t, err)
			defer cnt1.close()

			time.Sleep(1 * time.Second)

			cnt2, err := newContainer("ffmpeg", "dest", []string{
				"-rtsp_transport", "udp",
				"-i", "rtsp://" + ownDockerIP + ":8554/path1",
				"-vframes", "1",
				"-f", "image2",
				"-y", "/dev/null",
			})
			require.NoError(t, err)
			defer cnt2.close()
			require.Equal(t, 0, cnt2.wait())
		})
	}
}

func TestRTSPRunOnDemand(t *testing.T) {
	doneFile := filepath.Join(os.TempDir(), "ondemand_done")
	onDemandFile, err := writeTempFile([]byte(fmt.Sprintf(`#!/bin/sh
trap 'touch %s; [ -z "$(jobs -p)" ] || kill $(jobs -p)' INT
ffmpeg -hide_banner -loglevel error -re -i testimages/ffmpeg/emptyvideo.ts -c copy -f rtsp rtsp://localhost:$RTSP_PORT/$RTSP_PATH &
wait
`, doneFile)))
	require.NoError(t, err)
	defer os.Remove(onDemandFile)

	err = os.Chmod(onDemandFile, 0755)
	require.NoError(t, err)

	t.Run("describe", func(t *testing.T) {
		defer os.Remove(doneFile)

		p1, ok := testProgram(fmt.Sprintf("rtmpDisable: yes\n"+
			"paths:\n"+
			"  all:\n"+
			"    runOnDemand: %s\n"+
			"    runOnDemandCloseAfter: 2s\n", onDemandFile))
		require.Equal(t, true, ok)
		defer p1.close()

		func() {
			conn, err := net.Dial("tcp", "127.0.0.1:8554")
			require.NoError(t, err)
			defer conn.Close()
			bconn := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

			err = base.Request{
				Method: base.Describe,
				URL:    base.MustParseURL("rtsp://localhost:8554/ondemand"),
				Header: base.Header{
					"CSeq": base.HeaderValue{"1"},
				},
			}.Write(bconn.Writer)
			require.NoError(t, err)

			var res base.Response
			err = res.Read(bconn.Reader)
			require.NoError(t, err)
			require.Equal(t, base.StatusOK, res.StatusCode)
		}()

		for {
			_, err := os.Stat(doneFile)
			if err == nil {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
	})

	t.Run("describe and setup", func(t *testing.T) {
		defer os.Remove(doneFile)

		p1, ok := testProgram(fmt.Sprintf("rtmpDisable: yes\n"+
			"paths:\n"+
			"  all:\n"+
			"    runOnDemand: %s\n"+
			"    runOnDemandCloseAfter: 2s\n", onDemandFile))
		require.Equal(t, true, ok)
		defer p1.close()

		func() {
			conn, err := net.Dial("tcp", "127.0.0.1:8554")
			require.NoError(t, err)
			defer conn.Close()
			bconn := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

			err = base.Request{
				Method: base.Describe,
				URL:    base.MustParseURL("rtsp://localhost:8554/ondemand"),
				Header: base.Header{
					"CSeq": base.HeaderValue{"1"},
				},
			}.Write(bconn.Writer)
			require.NoError(t, err)

			var res base.Response
			err = res.Read(bconn.Reader)
			require.NoError(t, err)
			require.Equal(t, base.StatusOK, res.StatusCode)

			err = base.Request{
				Method: base.Setup,
				URL:    base.MustParseURL("rtsp://localhost:8554/ondemand/trackID=0"),
				Header: base.Header{
					"CSeq": base.HeaderValue{"2"},
					"Transport": headers.Transport{
						Protocol: gortsplib.StreamProtocolTCP,
						Delivery: func() *base.StreamDelivery {
							v := base.StreamDeliveryUnicast
							return &v
						}(),
						Mode: func() *headers.TransportMode {
							v := headers.TransportModePlay
							return &v
						}(),
						InterleavedIds: &[2]int{0, 1},
					}.Write(),
				},
			}.Write(bconn.Writer)
			require.NoError(t, err)

			err = res.Read(bconn.Reader)
			require.NoError(t, err)
			require.Equal(t, base.StatusOK, res.StatusCode)
		}()

		for {
			_, err := os.Stat(doneFile)
			if err == nil {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
	})

	t.Run("setup", func(t *testing.T) {
		defer os.Remove(doneFile)

		p1, ok := testProgram(fmt.Sprintf("rtmpDisable: yes\n"+
			"paths:\n"+
			"  all:\n"+
			"    runOnDemand: %s\n"+
			"    runOnDemandCloseAfter: 2s\n", onDemandFile))
		require.Equal(t, true, ok)
		defer p1.close()

		func() {
			conn, err := net.Dial("tcp", "127.0.0.1:8554")
			require.NoError(t, err)
			defer conn.Close()
			bconn := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

			err = base.Request{
				Method: base.Setup,
				URL:    base.MustParseURL("rtsp://localhost:8554/ondemand/trackID=0"),
				Header: base.Header{
					"CSeq": base.HeaderValue{"1"},
					"Transport": headers.Transport{
						Protocol: gortsplib.StreamProtocolTCP,
						Delivery: func() *base.StreamDelivery {
							v := base.StreamDeliveryUnicast
							return &v
						}(),
						Mode: func() *headers.TransportMode {
							v := headers.TransportModePlay
							return &v
						}(),
						InterleavedIds: &[2]int{0, 1},
					}.Write(),
				},
			}.Write(bconn.Writer)
			require.NoError(t, err)

			var res base.Response
			err = res.Read(bconn.Reader)
			require.NoError(t, err)
			require.Equal(t, base.StatusOK, res.StatusCode)
		}()

		for {
			_, err := os.Stat(doneFile)
			if err == nil {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
	})
}
