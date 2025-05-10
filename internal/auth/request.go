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
	Action conf.AuthAction

	// only for ActionPublish, ActionRead, ActionPlayback
	Path     string
	Query    string
	Protocol Protocol
	ID       *uuid.UUID

	Credentials      *Credentials
	IP               net.IP
	CustomVerifyFunc func(expectedUser string, expectedPass string) bool
}
