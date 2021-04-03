package rtmp

import (
	"github.com/notedit/rtmp/format/rtmp"
)

// Dial connects to a server in reading mode.
func Dial(address string) (*Conn, error) {
	rconn, nconn, err := rtmp.NewClient().Dial(address, rtmp.PrepareReading)
	if err != nil {
		return nil, err
	}

	return NewConn(rconn, nconn), nil
}
