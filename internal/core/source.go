package core

// source is an entity that can provide a stream, statically or dynamically.
type source interface {
	OnSourceAPIDescribe() interface{}
}

// sourceStatic is an entity that can provide a static stream.
type sourceStatic interface {
	source
	Close()
}
