package core

import (
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// sourceRedirect is a source that redirects to another one.
type sourceRedirect struct{}

func (*sourceRedirect) Log(logger.Level, string, ...any) {
}

// APISourceDescribe implements source.
func (*sourceRedirect) APISourceDescribe() *defs.APIPathSource {
	return &defs.APIPathSource{
		Type: "redirect",
		ID:   "",
	}
}
