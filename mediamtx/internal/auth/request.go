package auth

import (
	"net"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/google/uuid"
)

// Protocol is a protocol.
type Protocol string

// protocols.
const (
	ProtocolRTSP   Protocol = "rtsp"
	ProtocolRTMP   Protocol = "rtmp"
	ProtocolHLS    Protocol = "hls"
	ProtocolWebRTC Protocol = "webrtc"
	ProtocolSRT    Protocol = "srt"
)

// Request is an authentication request.
type Request struct {
	Action           conf.AuthAction
	Path             string // only for ActionPublish, ActionRead, ActionPlayback
	Query            string
	Protocol         Protocol   // only for ActionPublish, ActionRead
	ID               *uuid.UUID // only for ActionPublish, ActionRead
	Credentials      *Credentials
	IP               net.IP
	CustomVerifyFunc func(expectedUser string, expectedPass string) bool
}
