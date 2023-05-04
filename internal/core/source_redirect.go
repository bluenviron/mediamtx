package core

import (
	"github.com/aler9/mediamtx/internal/logger"
)

// sourceRedirect is a source that redirects to another one.
type sourceRedirect struct{}

func (*sourceRedirect) Log(logger.Level, string, ...interface{}) {
}

// apiSourceDescribe implements source.
func (*sourceRedirect) apiSourceDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"redirect"}
}
