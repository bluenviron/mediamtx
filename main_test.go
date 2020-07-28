package main

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var ownDockerIp = func() string {
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
		"--format={{.State.ExitCode}}").Output()
	code, _ := strconv.ParseInt(string(out[:len(out)-1]), 10, 64)
	return int(code)
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
			p, err := newProgram([]string{}, bytes.NewBuffer(nil))
			require.NoError(t, err)
			defer p.close()

			time.Sleep(1 * time.Second)

			switch conf.publishSoft {
			case "ffmpeg":
				cnt1, err := newContainer("ffmpeg", "publish", []string{
					"-re",
					"-stream_loop", "-1",
					"-i", "/emptyvideo.ts",
					"-c", "copy",
					"-f", "rtsp",
					"-rtsp_transport", conf.publishProto,
					"rtsp://" + ownDockerIp + ":8554/teststream",
				})
				require.NoError(t, err)
				defer cnt1.close()

			default:
				cnt1, err := newContainer("gstreamer", "source", []string{
					"filesrc location=emptyvideo.ts ! tsdemux ! rtspclientsink " +
						"location=rtsp://" + ownDockerIp + ":8554/teststream protocols=" + conf.publishProto + " latency=0",
				})
				require.NoError(t, err)
				defer cnt1.close()
			}

			time.Sleep(1 * time.Second)

			cnt2, err := newContainer("ffmpeg", "read", []string{
				"-rtsp_transport", "udp",
				"-i", "rtsp://" + ownDockerIp + ":8554/teststream",
				"-vframes", "1",
				"-f", "image2",
				"-y", "/dev/null",
			})
			require.NoError(t, err)
			defer cnt2.close()

			code := cnt2.wait()
			require.Equal(t, 0, code)
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
			p, err := newProgram([]string{}, bytes.NewBuffer(nil))
			require.NoError(t, err)
			defer p.close()

			time.Sleep(1 * time.Second)

			cnt1, err := newContainer("ffmpeg", "publish", []string{
				"-re",
				"-stream_loop", "-1",
				"-i", "/emptyvideo.ts",
				"-c", "copy",
				"-f", "rtsp",
				"-rtsp_transport", "udp",
				"rtsp://" + ownDockerIp + ":8554/teststream",
			})
			require.NoError(t, err)
			defer cnt1.close()

			time.Sleep(1 * time.Second)

			switch conf.readSoft {
			case "ffmpeg":
				cnt2, err := newContainer("ffmpeg", "read", []string{
					"-rtsp_transport", conf.readProto,
					"-i", "rtsp://" + ownDockerIp + ":8554/teststream",
					"-vframes", "1",
					"-f", "image2",
					"-y", "/dev/null",
				})
				require.NoError(t, err)
				defer cnt2.close()

				code := cnt2.wait()
				require.Equal(t, 0, code)

			default:
				args := []string{}
				if conf.readProto == "tcp" {
					args = append(args, "--rtsp-tcp")
				}
				args = append(args, "rtsp://"+ownDockerIp+":8554/teststream")

				cnt2, err := newContainer("vlc", "dest", args)
				require.NoError(t, err)
				defer cnt2.close()

				code := cnt2.wait()
				require.Equal(t, 0, code)
			}
		})
	}
}

func TestAuth(t *testing.T) {
	t.Run("publish", func(t *testing.T) {
		stdin := []byte("\n" +
			"paths:\n" +
			"  all:\n" +
			"    publishUser: testuser\n" +
			"    publishPass: testpass\n" +
			"    publishIps: [172.17.0.0/16]\n")
		p, err := newProgram([]string{"stdin"}, bytes.NewBuffer(stdin))
		require.NoError(t, err)
		defer p.close()

		time.Sleep(1 * time.Second)

		cnt1, err := newContainer("ffmpeg", "source", []string{
			"-re",
			"-stream_loop", "-1",
			"-i", "/emptyvideo.ts",
			"-c", "copy",
			"-f", "rtsp",
			"-rtsp_transport", "udp",
			"rtsp://testuser:testpass@" + ownDockerIp + ":8554/teststream",
		})
		require.NoError(t, err)
		defer cnt1.close()

		time.Sleep(1 * time.Second)

		cnt2, err := newContainer("ffmpeg", "dest", []string{
			"-rtsp_transport", "udp",
			"-i", "rtsp://" + ownDockerIp + ":8554/teststream",
			"-vframes", "1",
			"-f", "image2",
			"-y", "/dev/null",
		})
		require.NoError(t, err)
		defer cnt2.close()

		code := cnt2.wait()
		require.Equal(t, 0, code)
	})

	for _, soft := range []string{
		"ffmpeg",
		"vlc",
	} {
		t.Run("read_"+soft, func(t *testing.T) {
			stdin := []byte("\n" +
				"paths:\n" +
				"  all:\n" +
				"    readUser: testuser\n" +
				"    readPass: testpass\n" +
				"    readIps: [172.17.0.0/16]\n")
			p, err := newProgram([]string{"stdin"}, bytes.NewBuffer(stdin))
			require.NoError(t, err)
			defer p.close()

			time.Sleep(1 * time.Second)

			cnt1, err := newContainer("ffmpeg", "source", []string{
				"-re",
				"-stream_loop", "-1",
				"-i", "/emptyvideo.ts",
				"-c", "copy",
				"-f", "rtsp",
				"-rtsp_transport", "udp",
				"rtsp://" + ownDockerIp + ":8554/teststream",
			})
			require.NoError(t, err)
			defer cnt1.close()

			time.Sleep(1 * time.Second)

			if soft == "ffmpeg" {
				cnt2, err := newContainer("ffmpeg", "dest", []string{
					"-rtsp_transport", "udp",
					"-i", "rtsp://testuser:testpass@" + ownDockerIp + ":8554/teststream",
					"-vframes", "1",
					"-f", "image2",
					"-y", "/dev/null",
				})
				require.NoError(t, err)
				defer cnt2.close()

				code := cnt2.wait()
				require.Equal(t, 0, code)

			} else {
				cnt2, err := newContainer("vlc", "dest",
					[]string{"rtsp://testuser:testpass@" + ownDockerIp + ":8554/teststream"})
				require.NoError(t, err)
				defer cnt2.close()

				code := cnt2.wait()
				require.Equal(t, 0, code)
			}
		})
	}
}

func TestProxy(t *testing.T) {
	for _, proto := range []string{
		"udp",
		"tcp",
	} {
		t.Run(proto, func(t *testing.T) {
			stdin := []byte("\n" +
				"paths:\n" +
				"  all:\n" +
				"    readUser: testuser\n" +
				"    readPass: testpass\n")
			p1, err := newProgram([]string{"stdin"}, bytes.NewBuffer(stdin))
			require.NoError(t, err)
			defer p1.close()

			time.Sleep(1 * time.Second)

			cnt1, err := newContainer("ffmpeg", "source", []string{
				"-re",
				"-stream_loop", "-1",
				"-i", "/emptyvideo.ts",
				"-c", "copy",
				"-f", "rtsp",
				"-rtsp_transport", "udp",
				"rtsp://" + ownDockerIp + ":8554/teststream",
			})
			require.NoError(t, err)
			defer cnt1.close()

			time.Sleep(1 * time.Second)

			stdin = []byte("\n" +
				"rtspPort: 8555\n" +
				"rtpPort: 8100\n" +
				"rtcpPort: 8101\n" +
				"\n" +
				"paths:\n" +
				"  proxied:\n" +
				"    source: rtsp://testuser:testpass@localhost:8554/teststream\n" +
				"    sourceProtocol: " + proto + "\n")
			p2, err := newProgram([]string{"stdin"}, bytes.NewBuffer(stdin))
			require.NoError(t, err)
			defer p2.close()

			time.Sleep(1 * time.Second)

			cnt2, err := newContainer("ffmpeg", "dest", []string{
				"-rtsp_transport", "udp",
				"-i", "rtsp://" + ownDockerIp + ":8555/proxied",
				"-vframes", "1",
				"-f", "image2",
				"-y", "/dev/null",
			})
			require.NoError(t, err)
			defer cnt2.close()

			code := cnt2.wait()
			require.Equal(t, 0, code)
		})
	}
}
