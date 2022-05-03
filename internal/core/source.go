package core

// source is an entity that can provide a stream.
// it can be:
// - a publisher
// - a static source
// - a redirect source
type source interface {
	onSourceAPIDescribe() interface{}
}

// sourceStatic is an entity that can provide a static stream.
type sourceStatic interface {
	source
	close()
}

// sourceRedirect is a source that redirects to another one.
type sourceRedirect struct{}

// onSourceAPIDescribe implements source.
func (*sourceRedirect) onSourceAPIDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"redirect"}
}
