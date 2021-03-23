package streamproc

import (
	"time"

	"github.com/aler9/gortsplib"
	"github.com/pion/rtp"
)

// TrackStartingPoint is the starting point of a track.
type TrackStartingPoint struct {
	Filled  bool // used to avoid mutexes
	RTPTime uint32
	NTPTime time.Time
}

// Path is implemented by path.path.
type Path interface {
	OnSPSetStartingPoint(SetStartingPointReq)
	OnSPFrame(int, gortsplib.StreamType, []byte)
}

// SetStartingPointReq is a set starting point request.
type SetStartingPointReq struct {
	SP            *StreamProc
	TrackID       int
	StartingPoint TrackStartingPoint
}

// StreamProc is a stream processor, an intermediate layer between a source and a path.
type StreamProc struct {
	path           Path
	startingPoints []TrackStartingPoint
}

// New allocates a StreamProc.
func New(path Path, tracksLen int) *StreamProc {
	return &StreamProc{
		path:           path,
		startingPoints: make([]TrackStartingPoint, tracksLen),
	}
}

// OnFrame processes a frame.
func (sp *StreamProc) OnFrame(trackID int, streamType gortsplib.StreamType, payload []byte) {
	if streamType == gortsplib.StreamTypeRTP &&
		!sp.startingPoints[trackID].Filled {
		pkt := rtp.Packet{}
		err := pkt.Unmarshal(payload)
		if err != nil {
			return
		}

		sp.startingPoints[trackID].Filled = true
		sp.startingPoints[trackID].RTPTime = pkt.Timestamp
		sp.startingPoints[trackID].NTPTime = time.Now()

		sp.path.OnSPSetStartingPoint(SetStartingPointReq{
			SP:            sp,
			TrackID:       trackID,
			StartingPoint: sp.startingPoints[trackID],
		})
	}

	sp.path.OnSPFrame(trackID, streamType, payload)
}
