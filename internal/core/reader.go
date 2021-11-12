package core

// reader is an entity that can read a stream.
type reader interface {
	close()
	onReaderAccepted()
	onReaderPacketRTP(int, []byte)
	onReaderPacketRTCP(int, []byte)
	onReaderAPIDescribe() interface{}
}
