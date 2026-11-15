package rtsp

import (
	"net/url"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
)

// TunnelURL splits a rtsp, rtsps, rtsp+http, rtsps+http, rtsp+ws or rtsps+ws URL
// into a rtsp or rtsps URL and the tunnel.
func TunnelURL(u *url.URL) (*base.URL, gortsplib.Tunnel, error) {
	tunnel := gortsplib.TunnelNone

	switch u.Scheme {
	case "rtsp+http", "rtsps+http":
		tunnel = gortsplib.TunnelHTTP
	case "rtsp+ws", "rtsps+ws":
		tunnel = gortsplib.TunnelWebSocket
	}

	u2 := *u

	switch u.Scheme {
	case "rtsp", "rtsp+http", "rtsp+ws":
		u2.Scheme = "rtsp"
	default:
		u2.Scheme = "rtsps"
	}

	ru, err := base.ParseURL(u2.String())
	if err != nil {
		return nil, gortsplib.TunnelNone, err
	}

	return ru, tunnel, nil
}
