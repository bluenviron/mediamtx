package streamproc

import (
	"github.com/aler9/gortsplib"
	"github.com/pion/rtp"

	"github.com/aler9/rtsp-simple-server/internal/source"
)

// Path is implemented by path.path.
type Path interface {
	OnSetStartingPoint(source.SetStartingPointReq)
	OnFrame(int, gortsplib.StreamType, []byte)
}

// StreamProc is a stream processor, an intermediate layer between a source and a path.
type StreamProc struct {
	source         source.Source
	path           Path
	startingPoints []source.TrackStartingPoint
}

// New allocates a StreamProc.
func New(source source.Source, path Path, startingPoints []source.TrackStartingPoint) *StreamProc {
	return &StreamProc{
		source:         source,
		path:           path,
		startingPoints: startingPoints,
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
		sp.startingPoints[trackID].SequenceNumber = pkt.SequenceNumber
		sp.startingPoints[trackID].Timestamp = pkt.Timestamp

		sp.path.OnSetStartingPoint(source.SetStartingPointReq{
			Source:        sp.source,
			TrackID:       trackID,
			StartingPoint: sp.startingPoints[trackID],
		})
	}

	sp.path.OnFrame(trackID, streamType, payload)
}
