package streamproc

import (
	"encoding/binary"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
)

// Path is implemented by path.path.
type Path interface {
	OnSPFrame(int, gortsplib.StreamType, []byte)
}

// TrackInfo contains infos about a track.
type TrackInfo struct {
	LastSequenceNumber uint16
	LastTimeRTP        uint32
	LastTimeNTP        int64
	LastSSRC           uint32
}

type track struct {
	lastSequenceNumber uint32
	lastTimeRTP        uint32
	lastTimeNTP        int64
	lastSSRC           uint32
}

// StreamProc is a stream processor, an intermediate layer between a source and a path.
type StreamProc struct {
	path   Path
	tracks []*track
}

// New allocates a StreamProc.
func New(path Path, tracksLen int) *StreamProc {
	sp := &StreamProc{
		path: path,
	}

	sp.tracks = make([]*track, tracksLen)
	for i := range sp.tracks {
		sp.tracks[i] = &track{}
	}

	return sp
}

// TrackInfos returns infos about the tracks of the stream.
func (sp *StreamProc) TrackInfos() []TrackInfo {
	ret := make([]TrackInfo, len(sp.tracks))
	for trackID, track := range sp.tracks {
		ret[trackID] = TrackInfo{
			LastSequenceNumber: uint16(atomic.LoadUint32(&track.lastSequenceNumber)),
			LastTimeRTP:        atomic.LoadUint32(&track.lastTimeRTP),
			LastTimeNTP:        atomic.LoadInt64(&track.lastTimeNTP),
			LastSSRC:           atomic.LoadUint32(&track.lastSSRC),
		}
	}
	return ret
}

// OnFrame processes a frame.
func (sp *StreamProc) OnFrame(trackID int, streamType gortsplib.StreamType, payload []byte) {
	if streamType == gortsplib.StreamTypeRTP && len(payload) >= 8 {
		track := sp.tracks[trackID]

		sequenceNumber := binary.BigEndian.Uint16(payload[2:4])
		atomic.StoreUint32(&track.lastSequenceNumber, uint32(sequenceNumber))

		timestamp := binary.BigEndian.Uint32(payload[4:8])
		atomic.StoreUint32(&track.lastTimeRTP, timestamp)
		atomic.StoreInt64(&track.lastTimeNTP, time.Now().Unix())

		ssrc := binary.BigEndian.Uint32(payload[8:12])
		atomic.StoreUint32(&track.lastSSRC, ssrc)
	}

	sp.path.OnSPFrame(trackID, streamType, payload)
}
