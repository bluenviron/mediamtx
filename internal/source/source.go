package source

import (
	"github.com/aler9/gortsplib"
)

// Source is implemented by all sources (clients and external sources).
type Source interface {
	IsSource()
}

// ExtSource is implemented by all external sources.
type ExtSource interface {
	IsSource()
	IsExtSource()
	Close()
}

// StreamProc is implemented by streamproc.StreamProc.
type StreamProc interface {
	OnFrame(int, gortsplib.StreamType, []byte)
}

// ExtSetReadyRes is a set ready response.
type ExtSetReadyRes struct {
	SP StreamProc
}

// ExtSetReadyReq is a set ready request.
type ExtSetReadyReq struct {
	Tracks gortsplib.Tracks
	Res    chan ExtSetReadyRes
}

// ExtSetNotReadyReq is a set not ready request.
type ExtSetNotReadyReq struct {
	Res chan struct{}
}
