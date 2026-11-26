package rtsp

import (
	"net/url"
	"testing"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/stretchr/testify/require"
)

func TestTunnelURL(t *testing.T) {
	for _, ca := range []struct {
		in     string
		out    string
		tunnel gortsplib.Tunnel
	}{
		{"rtsp://user:pass@host:8554/path?key=val", "rtsp://user:pass@host:8554/path?key=val", gortsplib.TunnelNone},
		{"rtsps://host/path", "rtsps://host/path", gortsplib.TunnelNone},
		{"rtsp+http://host/path", "rtsp://host/path", gortsplib.TunnelHTTP},
		{"rtsps+http://host/path", "rtsps://host/path", gortsplib.TunnelHTTP},
		{"rtsp+ws://host/path", "rtsp://host/path", gortsplib.TunnelWebSocket},
		{"rtsps+ws://host/path", "rtsps://host/path", gortsplib.TunnelWebSocket},
	} {
		t.Run(ca.in, func(t *testing.T) {
			u0, err := url.Parse(ca.in)
			require.NoError(t, err)

			u, tunnel, err := TunnelURL(u0)
			require.NoError(t, err)
			require.Equal(t, ca.out, u.String())
			require.Equal(t, ca.tunnel, tunnel)
		})
	}
}
