package source

import (
	"github.com/aler9/gortsplib"
)

// TrackStartingPoint is the starting point of a track.
type TrackStartingPoint struct {
	Filled         bool // used to avoid mutexes
	SequenceNumber uint16
	Timestamp      uint32
}

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

// SetStartingPointReq is a set starting point request.
type SetStartingPointReq struct {
	Source        Source
	TrackID       int
	StartingPoint TrackStartingPoint
}

// ExtSetReadyReq is a set ready request.
type ExtSetReadyReq struct {
	Tracks         gortsplib.Tracks
	StartingPoints []TrackStartingPoint
	Res            chan struct{}
}

// ExtSetNotReadyReq is a set not ready request.
type ExtSetNotReadyReq struct {
	Res chan struct{}
}
