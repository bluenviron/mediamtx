package core

import (
	"fmt"

	"github.com/aler9/gortsplib"
)

type streamTrack interface {
	writeData(*data)
}

func newStreamTrack(track gortsplib.Track, generateRTPPackets bool, writeDataInner func(*data)) (streamTrack, error) {
	switch ttrack := track.(type) {
	case *gortsplib.TrackH264:
		return newStreamTrackH264(ttrack, generateRTPPackets, writeDataInner), nil

	case *gortsplib.TrackMPEG4Audio:
		return newStreamTrackMPEG4Audio(ttrack, generateRTPPackets, writeDataInner), nil

	default:
		if generateRTPPackets {
			return nil, fmt.Errorf("we don't know how to generate RTP packets of track %+v", track)
		}
		return newStreamTrackGeneric(track, writeDataInner), nil
	}
}
