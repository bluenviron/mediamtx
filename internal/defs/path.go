package defs

import (
	"github.com/bluenviron/gortsplib/v4/pkg/description"

	"github.com/bluenviron/mediamtx/internal/stream"
)

// PathSourceStaticSetReadyRes is a set ready response to a static source.
type PathSourceStaticSetReadyRes struct {
	Stream *stream.Stream
	Err    error
}

// PathSourceStaticSetReadyReq is a set ready request from a static source.
type PathSourceStaticSetReadyReq struct {
	Desc               *description.Session
	GenerateRTPPackets bool
	Res                chan PathSourceStaticSetReadyRes
}

// PathSourceStaticSetNotReadyReq is a set not ready request from a static source.
type PathSourceStaticSetNotReadyReq struct {
	Res chan struct{}
}
