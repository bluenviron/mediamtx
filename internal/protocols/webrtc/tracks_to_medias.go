package webrtc

import (
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
)

// TracksToMedias converts WebRTC tracks into a media description.
func TracksToMedias(tracks []*IncomingTrack) []*description.Media {
	ret := make([]*description.Media, len(tracks))

	for i, track := range tracks {
		ret[i] = &description.Media{
			Type:    track.typ,
			Formats: []format.Format{track.format},
		}
	}

	return ret
}
