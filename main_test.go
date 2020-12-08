package main

import (
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/stretchr/testify/require"

	"github.com/aler9/rtsp-simple-server/internal/conf"
)

var ownDockerIP = func() string {
	out, err := exec.Command("docker", "network", "inspect", "bridge",
		"-f", "{{range .IPAM.Config}}{{.Subnet}}{{end}}").Output()
	if err != nil {
		panic(err)
	}

	_, ipnet, err := net.ParseCIDR(string(out[:len(out)-1]))
	if err != nil {
		panic(err)
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		panic(err)
	}

	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if v, ok := addr.(*net.IPNet); ok {
				if ipnet.Contains(v.IP) {
					return v.IP.String()
				}
			}
		}
	}

	panic("IP not found")
}()

type container struct {
	name string
}

func newContainer(image string, name string, args []string) (*container, error) {
	c := &container{
		name: name,
	}

	exec.Command("docker", "kill", "rtsp-simple-server-test-"+name).Run()
	exec.Command("docker", "wait", "rtsp-simple-server-test-"+name).Run()

	cmd := []string{"docker", "run",
		"--name=rtsp-simple-server-test-" + name,
		"rtsp-simple-server-test-" + image}
	cmd = append(cmd, args...)
	ecmd := exec.Command(cmd[0], cmd[1:]...)
	ecmd.Stdout = nil
	ecmd.Stderr = os.Stderr

	err := ecmd.Start()
	if err != nil {
		return nil, err
	}

	time.Sleep(1 * time.Second)

	return c, nil
}

func (c *container) close() {
	exec.Command("docker", "kill", "rtsp-simple-server-test-"+c.name).Run()
	exec.Command("docker", "wait", "rtsp-simple-server-test-"+c.name).Run()
	exec.Command("docker", "rm", "rtsp-simple-server-test-"+c.name).Run()
}

func (c *container) wait() int {
	exec.Command("docker", "wait", "rtsp-simple-server-test-"+c.name).Run()
	out, _ := exec.Command("docker", "inspect", "rtsp-simple-server-test-"+c.name,
		"-f", "{{.State.ExitCode}}").Output()
	code, _ := strconv.ParseInt(string(out[:len(out)-1]), 10, 64)
	return int(code)
}

func (c *container) ip() string {
	out, _ := exec.Command("docker", "inspect", "rtsp-simple-server-test-"+c.name,
		"-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}").Output()
	return string(out[:len(out)-1])
}

func testProgram(conf string) (*program, bool) {
	if conf == "" {
		return newProgram([]string{})
	}

	tmpf, err := ioutil.TempFile(os.TempDir(), "rtsp-")
	if err != nil {
		return nil, false
	}
	defer os.Remove(tmpf.Name())

	tmpf.WriteString(conf)
	tmpf.Close()

	return newProgram([]string{tmpf.Name()})
}

func TestEnvironment(t *testing.T) {
	// string
	os.Setenv("RTSP_RUNONCONNECT", "test=cmd")
	defer os.Unsetenv("RTSP_RUNONCONNECT")

	// int
	os.Setenv("RTSP_RTSPPORT", "8555")
	defer os.Unsetenv("RTSP_RTSPPORT")

	// bool
	os.Setenv("RTSP_METRICS", "yes")
	defer os.Unsetenv("RTSP_METRICS")

	// duration
	os.Setenv("RTSP_READTIMEOUT", "22s")
	defer os.Unsetenv("RTSP_READTIMEOUT")

	// slice
	os.Setenv("RTSP_LOGDESTINATIONS", "stdout,file")
	defer os.Unsetenv("RTSP_LOGDESTINATIONS")

	// map key
	os.Setenv("RTSP_PATHS_TEST2", "")
	defer os.Unsetenv("RTSP_PATHS_TEST2")

	// map values, "all" path
	os.Setenv("RTSP_PATHS_ALL_READUSER", "testuser")
	defer os.Unsetenv("RTSP_PATHS_ALL_READUSER")
	os.Setenv("RTSP_PATHS_ALL_READPASS", "testpass")
	defer os.Unsetenv("RTSP_PATHS_ALL_READPASS")

	// map values, generic path
	os.Setenv("RTSP_PATHS_CAM1_SOURCE", "rtsp://testing")
	defer os.Unsetenv("RTSP_PATHS_CAM1_SOURCE")
	os.Setenv("RTSP_PATHS_CAM1_SOURCEPROTOCOL", "tcp")
	defer os.Unsetenv("RTSP_PATHS_CAM1_SOURCEPROTOCOL")
	os.Setenv("RTSP_PATHS_CAM1_SOURCEONDEMAND", "yes")
	defer os.Unsetenv("RTSP_PATHS_CAM1_SOURCEONDEMAND")

	p, ok := testProgram("")
	require.Equal(t, true, ok)
	defer p.close()

	require.Equal(t, "test=cmd", p.conf.RunOnConnect)

	require.Equal(t, 8555, p.conf.RtspPort)

	require.Equal(t, true, p.conf.Metrics)

	require.Equal(t, 22*time.Second, p.conf.ReadTimeout)

	require.Equal(t, []string{"stdout", "file"}, p.conf.LogDestinations)

	pa, ok := p.conf.Paths["test2"]
	require.Equal(t, true, ok)
	require.Equal(t, &conf.PathConf{
		Source:                     "record",
		SourceOnDemandStartTimeout: 10 * time.Second,
		SourceOnDemandCloseAfter:   10 * time.Second,
		RunOnDemandStartTimeout:    10 * time.Second,
		RunOnDemandCloseAfter:      10 * time.Second,
	}, pa)

	pa, ok = p.conf.Paths["~^.*$"]
	require.Equal(t, true, ok)
	require.Equal(t, &conf.PathConf{
		Regexp:                     regexp.MustCompile("^.*$"),
		Source:                     "record",
		SourceProtocol:             "automatic",
		SourceOnDemandStartTimeout: 10 * time.Second,
		SourceOnDemandCloseAfter:   10 * time.Second,
		ReadUser:                   "testuser",
		ReadPass:                   "testpass",
		RunOnDemandStartTimeout:    10 * time.Second,
		RunOnDemandCloseAfter:      10 * time.Second,
	}, pa)

	pa, ok = p.conf.Paths["cam1"]
	require.Equal(t, true, ok)
	require.Equal(t, &conf.PathConf{
		Source:         "rtsp://testing",
		SourceProtocol: "tcp",
		SourceProtocolParsed: func() *gortsplib.StreamProtocol {
			v := gortsplib.StreamProtocolTCP
			return &v
		}(),
		SourceOnDemand:             true,
		SourceOnDemandStartTimeout: 10 * time.Second,
		SourceOnDemandCloseAfter:   10 * time.Second,
		RunOnDemandStartTimeout:    10 * time.Second,
		RunOnDemandCloseAfter:      10 * time.Second,
	}, pa)
}

func TestEnvironmentNoFile(t *testing.T) {
	os.Setenv("RTSP_PATHS_CAM1_SOURCE", "rtsp://testing")
	defer os.Unsetenv("RTSP_PATHS_CAM1_SOURCE")

	p, ok := testProgram("{}")
	require.Equal(t, true, ok)
	defer p.close()

	pa, ok := p.conf.Paths["cam1"]
	require.Equal(t, true, ok)
	require.Equal(t, &conf.PathConf{
		Source:                     "rtsp://testing",
		SourceProtocol:             "automatic",
		SourceOnDemandStartTimeout: 10 * time.Second,
		SourceOnDemandCloseAfter:   10 * time.Second,
		RunOnDemandStartTimeout:    10 * time.Second,
		RunOnDemandCloseAfter:      10 * time.Second,
	}, pa)
}

func TestPublish(t *testing.T) {
	for _, conf := range []struct {
		publishSoft  string
		publishProto string
	}{
		{"ffmpeg", "udp"},
		{"ffmpeg", "tcp"},
		{"gstreamer", "udp"},
		{"gstreamer", "tcp"},
	} {
		t.Run(conf.publishSoft+"_"+conf.publishProto, func(t *testing.T) {
			p, ok := testProgram("readTimeout: 20s")
			require.Equal(t, true, ok)
			defer p.close()

			time.Sleep(1 * time.Second)

			switch conf.publishSoft {
			case "ffmpeg":
				cnt1, err := newContainer("ffmpeg", "source", []string{
					"-re",
					"-stream_loop", "-1",
					"-i", "/emptyvideo.ts",
					"-c", "copy",
					"-f", "rtsp",
					"-rtsp_transport", conf.publishProto,
					"rtsp://" + ownDockerIP + ":8554/teststream",
				})
				require.NoError(t, err)
				defer cnt1.close()

				time.Sleep(1 * time.Second)

			default:
				cnt1, err := newContainer("gstreamer", "source", []string{
					"filesrc location=emptyvideo.ts ! tsdemux ! queue ! video/x-h264 ! h264parse config-interval=1 ! rtspclientsink " +
						"location=rtsp://" + ownDockerIP + ":8554/teststream protocols=" + conf.publishProto + " latency=0 timeout=0 rtx-time=0",
				})
				require.NoError(t, err)
				defer cnt1.close()
			}

			time.Sleep(1 * time.Second)

			cnt2, err := newContainer("ffmpeg", "dest", []string{
				"-rtsp_transport", "udp",
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

func TestRead(t *testing.T) {
	for _, conf := range []struct {
		readSoft  string
		readProto string
	}{
		{"ffmpeg", "udp"},
		{"ffmpeg", "tcp"},
		{"vlc", "udp"},
		{"vlc", "tcp"},
	} {
		t.Run(conf.readSoft+"_"+conf.readProto, func(t *testing.T) {
			p, ok := testProgram("")
			require.Equal(t, true, ok)
			defer p.close()

			time.Sleep(1 * time.Second)

			cnt1, err := newContainer("ffmpeg", "source", []string{
				"-re",
				"-stream_loop", "-1",
				"-i", "/emptyvideo.ts",
				"-c", "copy",
				"-f", "rtsp",
				"-rtsp_transport", "udp",
				"rtsp://" + ownDockerIP + ":8554/teststream",
			})
			require.NoError(t, err)
			defer cnt1.close()

			time.Sleep(1 * time.Second)

			switch conf.readSoft {
			case "ffmpeg":
				cnt2, err := newContainer("ffmpeg", "dest", []string{
					"-rtsp_transport", conf.readProto,
					"-i", "rtsp://" + ownDockerIP + ":8554/teststream",
					"-vframes", "1",
					"-f", "image2",
					"-y", "/dev/null",
				})
				require.NoError(t, err)
				defer cnt2.close()

				require.Equal(t, 0, cnt2.wait())

			default:
				args := []string{}
				if conf.readProto == "tcp" {
					args = append(args, "--rtsp-tcp")
				}
				args = append(args, "rtsp://"+ownDockerIP+":8554/teststream")

				cnt2, err := newContainer("vlc", "dest", args)
				require.NoError(t, err)
				defer cnt2.close()

				require.Equal(t, 0, cnt2.wait())
			}
		})
	}
}

func TestTCPOnly(t *testing.T) {
	p, ok := testProgram("protocols: [tcp]\n")
	require.Equal(t, true, ok)
	defer p.close()

	time.Sleep(1 * time.Second)

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "/emptyvideo.ts",
		"-c", "copy",
		"-f", "rtsp",
		"-rtsp_transport", "tcp",
		"rtsp://" + ownDockerIP + ":8554/teststream",
	})
	require.NoError(t, err)
	defer cnt1.close()

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-rtsp_transport", "tcp",
		"-i", "rtsp://" + ownDockerIP + ":8554/teststream",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()

	require.Equal(t, 0, cnt2.wait())
}

func TestPathWithSlash(t *testing.T) {
	p, ok := testProgram("")
	require.Equal(t, true, ok)
	defer p.close()

	time.Sleep(1 * time.Second)

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "/emptyvideo.ts",
		"-c", "copy",
		"-f", "rtsp",
		"-rtsp_transport", "udp",
		"rtsp://" + ownDockerIP + ":8554/test/stream",
	})
	require.NoError(t, err)
	defer cnt1.close()

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
}

func TestPathWithQuery(t *testing.T) {
	p, ok := testProgram("")
	require.Equal(t, true, ok)
	defer p.close()

	time.Sleep(1 * time.Second)

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "/emptyvideo.ts",
		"-c", "copy",
		"-f", "rtsp",
		"-rtsp_transport", "udp",
		"rtsp://" + ownDockerIP + ":8554/test?param1=val&param2=val",
	})
	require.NoError(t, err)
	defer cnt1.close()

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-rtsp_transport", "udp",
		"-i", "rtsp://" + ownDockerIP + ":8554/test?param3=otherval",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()

	require.Equal(t, 0, cnt2.wait())
}

func TestAuth(t *testing.T) {
	t.Run("publish", func(t *testing.T) {
		p, ok := testProgram("paths:\n" +
			"  all:\n" +
			"    publishUser: testuser\n" +
			"    publishPass: test!$()*+.;<=>[]^_-{}\n" +
			"    publishIps: [172.17.0.0/16]\n")
		require.Equal(t, true, ok)
		defer p.close()

		time.Sleep(1 * time.Second)

		cnt1, err := newContainer("ffmpeg", "source", []string{
			"-re",
			"-stream_loop", "-1",
			"-i", "/emptyvideo.ts",
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
			p, ok := testProgram("paths:\n" +
				"  all:\n" +
				"    readUser: testuser\n" +
				"    readPass: test!$()*+.;<=>[]^_-{}\n" +
				"    readIps: [172.17.0.0/16]\n")
			require.Equal(t, true, ok)
			defer p.close()

			time.Sleep(1 * time.Second)

			cnt1, err := newContainer("ffmpeg", "source", []string{
				"-re",
				"-stream_loop", "-1",
				"-i", "/emptyvideo.ts",
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
}

func TestAuthIpFail(t *testing.T) {
	p, ok := testProgram("paths:\n" +
		"  all:\n" +
		"    publishIps: [127.0.0.1/32]\n")
	require.Equal(t, true, ok)
	defer p.close()

	time.Sleep(1 * time.Second)

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "/emptyvideo.ts",
		"-c", "copy",
		"-f", "rtsp",
		"-rtsp_transport", "udp",
		"rtsp://" + ownDockerIP + ":8554/test/stream",
	})
	require.NoError(t, err)
	defer cnt1.close()

	require.NotEqual(t, 0, cnt1.wait())
}

func TestSourceRtsp(t *testing.T) {
	for _, proto := range []string{
		"udp",
		"tcp",
	} {
		t.Run(proto, func(t *testing.T) {
			p1, ok := testProgram("paths:\n" +
				"  all:\n" +
				"    readUser: testuser\n" +
				"    readPass: testpass\n")
			require.Equal(t, true, ok)
			defer p1.close()

			time.Sleep(1 * time.Second)

			cnt1, err := newContainer("ffmpeg", "source", []string{
				"-re",
				"-stream_loop", "-1",
				"-i", "/emptyvideo.ts",
				"-c", "copy",
				"-f", "rtsp",
				"-rtsp_transport", "udp",
				"rtsp://" + ownDockerIP + ":8554/teststream",
			})
			require.NoError(t, err)
			defer cnt1.close()

			time.Sleep(1 * time.Second)

			p2, ok := testProgram("rtspPort: 8555\n" +
				"rtpPort: 8100\n" +
				"rtcpPort: 8101\n" +
				"\n" +
				"paths:\n" +
				"  proxied:\n" +
				"    source: rtsp://testuser:testpass@localhost:8554/teststream\n" +
				"    sourceProtocol: " + proto + "\n" +
				"    sourceOnDemand: yes\n")
			require.Equal(t, true, ok)
			defer p2.close()

			time.Sleep(1 * time.Second)

			cnt2, err := newContainer("ffmpeg", "dest", []string{
				"-rtsp_transport", "udp",
				"-i", "rtsp://" + ownDockerIP + ":8555/proxied",
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

func TestSourceRtmp(t *testing.T) {
	cnt1, err := newContainer("nginx-rtmp", "rtmpserver", []string{})
	require.NoError(t, err)
	defer cnt1.close()

	time.Sleep(1 * time.Second)

	cnt2, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "/emptyvideo.ts",
		"-c", "copy",
		"-f", "flv",
		"rtmp://" + cnt1.ip() + "/stream/test",
	})
	require.NoError(t, err)
	defer cnt2.close()

	time.Sleep(1 * time.Second)

	p, ok := testProgram("paths:\n" +
		"  proxied:\n" +
		"    source: rtmp://" + cnt1.ip() + "/stream/test\n" +
		"    sourceOnDemand: yes\n")
	require.Equal(t, true, ok)
	defer p.close()

	time.Sleep(1 * time.Second)

	cnt3, err := newContainer("ffmpeg", "dest", []string{
		"-rtsp_transport", "udp",
		"-i", "rtsp://" + ownDockerIP + ":8554/proxied",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt3.close()

	require.Equal(t, 0, cnt3.wait())
}

func TestRedirect(t *testing.T) {
	p1, ok := testProgram("paths:\n" +
		"  path1:\n" +
		"    source: redirect\n" +
		"    sourceRedirect: rtsp://" + ownDockerIP + ":8554/path2\n" +
		"  path2:\n")
	require.Equal(t, true, ok)
	defer p1.close()

	time.Sleep(1 * time.Second)

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "/emptyvideo.ts",
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

func TestFallback(t *testing.T) {
	p1, ok := testProgram("paths:\n" +
		"  path1:\n" +
		"    fallback: rtsp://" + ownDockerIP + ":8554/path2\n" +
		"  path2:\n")
	require.Equal(t, true, ok)
	defer p1.close()

	time.Sleep(1 * time.Second)

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "/emptyvideo.ts",
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

func TestRunOnDemand(t *testing.T) {
	p1, ok := testProgram("paths:\n" +
		"  all:\n" +
		"    runOnDemand: ffmpeg -hide_banner -loglevel error -re -i testimages/ffmpeg/emptyvideo.ts -c copy -f rtsp rtsp://localhost:$RTSP_PORT/$RTSP_PATH\n")
	require.Equal(t, true, ok)
	defer p1.close()

	time.Sleep(1 * time.Second)

	cnt1, err := newContainer("ffmpeg", "dest", []string{
		"-i", "rtsp://" + ownDockerIP + ":8554/ondemand",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt1.close()

	require.Equal(t, 0, cnt1.wait())
}

func TestHotReloading(t *testing.T) {
	confPath := filepath.Join(os.TempDir(), "rtsp-conf")

	err := ioutil.WriteFile(confPath, []byte("paths:\n"+
		"  test1:\n"+
		"    runOnDemand: ffmpeg -hide_banner -loglevel error -re -i testimages/ffmpeg/emptyvideo.ts -c copy -f rtsp rtsp://localhost:$RTSP_PORT/$RTSP_PATH\n"+
		"  test3:\n"+
		"    runOnInit: echo aaa\n"+
		"  test4:\n"+
		"    runOnInit: echo bbb\n"),
		0644)
	require.NoError(t, err)
	defer os.Remove(confPath)

	p, ok := newProgram([]string{confPath})
	require.Equal(t, true, ok)
	defer p.close()

	time.Sleep(1 * time.Second)

	func() {
		cnt1, err := newContainer("ffmpeg", "dest", []string{
			"-i", "rtsp://" + ownDockerIP + ":8554/test1",
			"-vframes", "1",
			"-f", "image2",
			"-y", "/dev/null",
		})
		require.NoError(t, err)
		defer cnt1.close()

		require.Equal(t, 0, cnt1.wait())
	}()

	err = ioutil.WriteFile(confPath, []byte("paths:\n"+
		"  test2:\n"+
		"    runOnDemand: ffmpeg -hide_banner -loglevel error -re -i testimages/ffmpeg/emptyvideo.ts -c copy -f rtsp rtsp://localhost:$RTSP_PORT/$RTSP_PATH\n"+
		"  test3:\n"+
		"  test4:\n"+
		"    runOnInit: echo bbb\n"),
		0644)
	require.NoError(t, err)

	time.Sleep(1 * time.Second)

	func() {
		cnt1, err := newContainer("ffmpeg", "dest", []string{
			"-i", "rtsp://" + ownDockerIP + ":8554/test1",
			"-vframes", "1",
			"-f", "image2",
			"-y", "/dev/null",
		})
		require.NoError(t, err)
		defer cnt1.close()

		require.Equal(t, 1, cnt1.wait())
	}()

	func() {
		cnt1, err := newContainer("ffmpeg", "dest", []string{
			"-i", "rtsp://" + ownDockerIP + ":8554/test2",
			"-vframes", "1",
			"-f", "image2",
			"-y", "/dev/null",
		})
		require.NoError(t, err)
		defer cnt1.close()

		require.Equal(t, 0, cnt1.wait())
	}()
}
