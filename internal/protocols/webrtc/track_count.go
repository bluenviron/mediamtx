package webrtc

import (
	"fmt"

	"github.com/pion/sdp/v3"
)

// TrackCount returns the track count.
func TrackCount(medias []*sdp.MediaDescription) (int, error) {
	videoTrack := false
	audioTrack := false
	trackCount := 0

	for _, media := range medias {
		switch media.MediaName.Media {
		case "video":
			if videoTrack {
				return 0, fmt.Errorf("only a single video and a single audio track are supported")
			}
			videoTrack = true

		case "audio":
			if audioTrack {
				return 0, fmt.Errorf("only a single video and a single audio track are supported")
			}
			audioTrack = true

		default:
			return 0, fmt.Errorf("unsupported media '%s'", media.MediaName.Media)
		}

		trackCount++
	}

	return trackCount, nil
}
