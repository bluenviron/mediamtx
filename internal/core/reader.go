package core

// reader is an entity that can read a stream.
type reader interface {
	close()
	onReaderAccepted()
	onReaderData(int, *data)
	onReaderAPIDescribe() interface{}
}
