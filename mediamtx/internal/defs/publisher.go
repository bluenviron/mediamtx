package defs

// Publisher is an entity that can publish a stream.
type Publisher interface {
	Source
	Close()
}
