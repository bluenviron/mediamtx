package core

// publisher is an entity that can publish a stream.
type publisher interface {
	source
	close()
}
