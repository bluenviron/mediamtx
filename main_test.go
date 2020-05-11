package main

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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

	cmd := []string{"docker", "run", "--network=host",
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
				"rtsp://localhost:8554/teststream",
			})
			require.NoError(t, err)
			defer cnt1.close()

			time.Sleep(1 * time.Second)

			cnt2, err := newContainer("ffmpeg", "dest", []string{
				"-hide_banner",
				"-loglevel", "panic",
				"-rtsp_transport", pair[1],
				"-i", "rtsp://localhost:8554/teststream",
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
		"rtsp://testuser:testpass@localhost:8554/teststream",
	})
	require.NoError(t, err)
	defer cnt1.close()

	time.Sleep(1 * time.Second)

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-hide_banner",
		"-loglevel", "panic",
		"-rtsp_transport", "udp",
		"-i", "rtsp://localhost:8554/teststream",
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
		"rtsp://localhost:8554/teststream",
	})
	require.NoError(t, err)
	defer cnt1.close()

	time.Sleep(1 * time.Second)

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-hide_banner",
		"-loglevel", "panic",
		"-rtsp_transport", "udp",
		"-i", "rtsp://testuser:testpass@localhost:8554/teststream",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()

	cnt2.wait()

	require.Equal(t, "all right\n", string(cnt2.stdout.Bytes()))
}
