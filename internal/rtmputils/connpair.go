package rtmputils

import (
	"net"

	"github.com/notedit/rtmp/format/rtmp"
)

// ConnPair contains a RTMP connection and a net connection.
type ConnPair struct {
	RConn *rtmp.Conn
	NConn net.Conn
}
