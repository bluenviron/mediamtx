package core

// source is an entity that can provide a stream, statically or dynamically.
type source interface {
	onSourceAPIDescribe() interface{}
}

// sourceStatic is an entity that can provide a static stream.
type sourceStatic interface {
	source
	close()
}
