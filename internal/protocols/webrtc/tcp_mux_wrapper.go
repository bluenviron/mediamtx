package webrtc

import (
	"net"

	"github.com/pion/ice/v4"
)

// TCPMuxWrapper is a wrapper around ice.TCPMux.
type TCPMuxWrapper struct {
	Mux ice.TCPMux
	Ln  net.Listener
}
