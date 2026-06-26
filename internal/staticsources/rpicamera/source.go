// Package rpicamera contains the Raspberry Pi Camera static source.
package rpicamera

import (
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
)

type parent interface {
	logger.Writer
	SetReady(req defs.PathSourceStaticSetReadyReq) defs.PathSourceStaticSetReadyRes
	SetNotReady(req defs.PathSourceStaticSetNotReadyReq)
	AddReader(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error)
}

// Source is a Raspberry Pi Camera static source.
type Source struct {
	RTPMaxPayloadSize int
	LogLevel          conf.LogLevel
	Parent            parent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...any) {
	s.Parent.Log(level, "[RPI Camera source] "+format, args...)
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() *defs.APIPathSource {
	return &defs.APIPathSource{
		Type: defs.APIPathSourceTypeRPICameraSource,
		ID:   "",
	}
}
