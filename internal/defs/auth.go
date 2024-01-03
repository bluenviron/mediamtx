package defs

// AuthProtocol is a authentication protocol.
type AuthProtocol string

// authentication protocols.
const (
	AuthProtocolRTSP   AuthProtocol = "rtsp"
	AuthProtocolRTMP   AuthProtocol = "rtmp"
	AuthProtocolHLS    AuthProtocol = "hls"
	AuthProtocolWebRTC AuthProtocol = "webrtc"
	AuthProtocolSRT    AuthProtocol = "srt"
)

// AuthenticationError is a authentication error.
type AuthenticationError struct {
	Message string
}

// Error implements the error interface.
func (e AuthenticationError) Error() string {
	return "authentication failed: " + e.Message
}
