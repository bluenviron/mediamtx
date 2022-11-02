package core

// reader is an entity that can read a stream.
type reader interface {
	close()
	onReaderData(data)
	apiReaderDescribe() interface{}
}
