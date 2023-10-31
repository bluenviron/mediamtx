package webrtc

import (
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
)

// TracksToMedias converts WebRTC tracks into a media description.
func TracksToMedias(tracks []*IncomingTrack) []*description.Media {
	ret := make([]*description.Media, len(tracks))

	for i, track := range tracks {
		forma := track.Format()

		var mediaType description.MediaType

		switch forma.(type) {
		case *format.AV1, *format.VP9, *format.VP8, *format.H264:
			mediaType = description.MediaTypeVideo

		default:
			mediaType = description.MediaTypeAudio
		}

		ret[i] = &description.Media{
			Type:    mediaType,
			Formats: []format.Format{forma},
		}
	}

	return ret
}
