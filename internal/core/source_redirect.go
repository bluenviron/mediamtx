package core

// sourceRedirect is a source that redirects to another one.
type sourceRedirect struct{}

// onSourceAPIDescribe implements source.
func (*sourceRedirect) onSourceAPIDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"redirect"}
}
