package streamproc

import (
	"encoding/binary"
	"sync"
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
	Initialized        bool
	LastSequenceNumber uint16
	RTPTime            uint32
	NTPTime            time.Time
}

type trackInfo struct {
	initialized        bool
	lastSequenceNumber uint32
	timeMutex          sync.Mutex
	rtpTime            uint32
	ntpTime            time.Time
}

// StreamProc is a stream processor, an intermediate layer between a source and a path.
type StreamProc struct {
	path       Path
	trackInfos []trackInfo
}

// New allocates a StreamProc.
func New(path Path, tracksLen int) *StreamProc {
	return &StreamProc{
		path:       path,
		trackInfos: make([]trackInfo, tracksLen),
	}
}

// TrackInfos returns infos about the tracks of the stream.
func (sp *StreamProc) TrackInfos() []TrackInfo {
	ret := make([]TrackInfo, len(sp.trackInfos))

	for trackID := range sp.trackInfos {
		sp.trackInfos[trackID].timeMutex.Lock()
		ret[trackID] = TrackInfo{
			Initialized:        sp.trackInfos[trackID].initialized,
			LastSequenceNumber: uint16(atomic.LoadUint32(&sp.trackInfos[trackID].lastSequenceNumber)),
			RTPTime:            sp.trackInfos[trackID].rtpTime,
			NTPTime:            sp.trackInfos[trackID].ntpTime,
		}
		sp.trackInfos[trackID].timeMutex.Unlock()
	}

	return ret
}

// OnFrame processes a frame.
func (sp *StreamProc) OnFrame(trackID int, streamType gortsplib.StreamType, payload []byte) {
	if streamType == gortsplib.StreamTypeRTP && len(payload) >= 8 {
		// store last sequence number
		sequenceNumber := binary.BigEndian.Uint16(payload[2 : 2+2])
		atomic.StoreUint32(&sp.trackInfos[trackID].lastSequenceNumber, uint32(sequenceNumber))

		// store time mapping
		if !sp.trackInfos[trackID].initialized {
			timestamp := binary.BigEndian.Uint32(payload[4 : 4+4])
			sp.trackInfos[trackID].timeMutex.Lock()
			sp.trackInfos[trackID].initialized = true
			sp.trackInfos[trackID].rtpTime = timestamp
			sp.trackInfos[trackID].ntpTime = time.Now()
			sp.trackInfos[trackID].timeMutex.Unlock()
		}
	}

	sp.path.OnSPFrame(trackID, streamType, payload)
}
