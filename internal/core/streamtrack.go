package core

import (
	"fmt"

	"github.com/aler9/gortsplib"
)

type streamTrack interface {
	onData(data, bool) error
}

func newStreamTrack(track gortsplib.Track, generateRTPPackets bool) (streamTrack, error) {
	switch ttrack := track.(type) {
	case *gortsplib.TrackH264:
		return newStreamTrackH264(ttrack, generateRTPPackets), nil

	case *gortsplib.TrackMPEG4Audio:
		return newStreamTrackMPEG4Audio(ttrack, generateRTPPackets), nil

	default:
		if generateRTPPackets {
			return nil, fmt.Errorf("we don't know how to generate RTP packets of track %+v", track)
		}
		return newStreamTrackGeneric(), nil
	}
}
