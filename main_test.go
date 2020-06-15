package main

import (
	"bytes"
	"net"
	"os"
	"os/exec"
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
	name   string
	stdout *bytes.Buffer
}

func newContainer(image string, name string, args []string) (*container, error) {
	c := &container{
		name:   name,
		stdout: bytes.NewBuffer(nil),
	}

	exec.Command("docker", "kill", "rtsp-simple-server-test-"+name).Run()
	exec.Command("docker", "wait", "rtsp-simple-server-test-"+name).Run()

	cmd := []string{"docker", "run",
		"--name=rtsp-simple-server-test-" + name,
		"rtsp-simple-server-test-" + image}
	cmd = append(cmd, args...)
	ecmd := exec.Command(cmd[0], cmd[1:]...)

	ecmd.Stdout = c.stdout
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

func (c *container) wait() {
	exec.Command("docker", "wait", "rtsp-simple-server-test-"+c.name).Run()
}

func TestProtocols(t *testing.T) {
	for _, pair := range [][2]string{
		{"udp", "udp"},
		{"udp", "tcp"},
		{"tcp", "udp"},
		{"tcp", "tcp"},
	} {
		t.Run(pair[0]+"_"+pair[1], func(t *testing.T) {
			p, err := newProgram([]string{})
			require.NoError(t, err)
			defer p.close()

			time.Sleep(1 * time.Second)

			cnt1, err := newContainer("ffmpeg", "source", []string{
				"-hide_banner",
				"-loglevel", "panic",
				"-re",
				"-stream_loop", "-1",
				"-i", "/emptyvideo.ts",
				"-c", "copy",
				"-f", "rtsp",
				"-rtsp_transport", pair[0],
				"rtsp://" + ownDockerIp + ":8554/teststream",
			})
			require.NoError(t, err)
			defer cnt1.close()

			time.Sleep(1 * time.Second)

			cnt2, err := newContainer("ffmpeg", "dest", []string{
				"-hide_banner",
				"-loglevel", "panic",
				"-rtsp_transport", pair[1],
				"-i", "rtsp://" + ownDockerIp + ":8554/teststream",
				"-vframes", "1",
				"-f", "image2",
				"-y", "/dev/null",
			})
			require.NoError(t, err)
			defer cnt2.close()

			cnt2.wait()

			require.Equal(t, "all right\n", string(cnt2.stdout.Bytes()))
		})
	}
}

func TestPublishAuth(t *testing.T) {
	p, err := newProgram([]string{
		"--publish-user=testuser",
		"--publish-pass=testpass",
		"--publish-ips=172.17.0.0/16",
	})
	require.NoError(t, err)
	defer p.close()

	time.Sleep(1 * time.Second)

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-hide_banner",
		"-loglevel", "panic",
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
		"-hide_banner",
		"-loglevel", "panic",
		"-rtsp_transport", "udp",
		"-i", "rtsp://" + ownDockerIp + ":8554/teststream",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()

	cnt2.wait()

	require.Equal(t, "all right\n", string(cnt2.stdout.Bytes()))
}

func TestReadAuth(t *testing.T) {
	p, err := newProgram([]string{
		"--read-user=testuser",
		"--read-pass=testpass",
		"--read-ips=172.17.0.0/16",
	})
	require.NoError(t, err)
	defer p.close()

	time.Sleep(1 * time.Second)

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-hide_banner",
		"-loglevel", "panic",
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

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-hide_banner",
		"-loglevel", "panic",
		"-rtsp_transport", "udp",
		"-i", "rtsp://testuser:testpass@" + ownDockerIp + ":8554/teststream",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()

	cnt2.wait()

	require.Equal(t, "all right\n", string(cnt2.stdout.Bytes()))
}
