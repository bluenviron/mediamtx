package core

import (
	"github.com/pion/rtp"
)

// reader is an entity that can read a stream.
type reader interface {
	close()
	onReaderAccepted()
	onReaderPacketRTP(int, *rtp.Packet)
	onReaderAPIDescribe() interface{}
}
