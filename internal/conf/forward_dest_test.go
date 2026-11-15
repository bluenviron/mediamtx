package conf_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
)

func TestForwardDestWithoutCredentials(t *testing.T) {
	for _, ca := range []struct {
		dest string
		out  string
	}{
		{"rtsp://user:pass@host:8554/$MTX_PATH", "rtsp://host:8554/$MTX_PATH"},
		{"rtmp://user@host/app", "rtmp://host/app"},
		{"rtsp://host/path", "rtsp://host/path"},
		{"srt://host:9000?streamid=publish:x", "srt://host:9000?streamid=publish:x"},
	} {
		t.Run(ca.dest, func(t *testing.T) {
			d := conf.ForwardDest{Dest: ca.dest, DestFingerprint: "fp"}
			require.Equal(t, conf.ForwardDest{Dest: ca.out, DestFingerprint: "fp"}, d.WithoutCredentials())
		})
	}
}
