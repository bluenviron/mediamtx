package defs

// Reader is an entity that can read a stream.
type Reader interface {
	Close()
	APIReaderDescribe() APIPathSourceOrReader
}
