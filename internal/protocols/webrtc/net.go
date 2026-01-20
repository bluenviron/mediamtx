package webrtc

import (
	"net"

	"github.com/bluenviron/gortsplib/v5/pkg/readbuffer"
	"github.com/pion/transport/v4"
	"github.com/pion/transport/v4/stdnet"
)

type webrtcNet struct {
	udpReadBufferSize int

	*stdnet.Net
}

func (n *webrtcNet) initialize() error {
	var err error
	n.Net, err = stdnet.NewNet()
	if err != nil {
		return err
	}

	return nil
}

func (n *webrtcNet) ListenUDP(network string, laddr *net.UDPAddr) (transport.UDPConn, error) {
	conn, err := n.Net.ListenUDP(network, laddr)
	if err != nil {
		return nil, err
	}

	if n.udpReadBufferSize != 0 {
		err = readbuffer.SetReadBuffer(conn.(*net.UDPConn), n.udpReadBufferSize)
		if err != nil {
			return nil, err
		}
	}

	return conn, nil
}
