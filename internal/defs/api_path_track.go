package defs

import (
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/formatlabel"
)

// APIPathTrack is a track.
type APIPathTrack struct {
	Codec      APIPathTrackCodec      `json:"codec"`
	CodecProps APIPathTrackCodecProps `json:"codecProps"`
}

func formatToTrack(forma format.Format) APIPathTrack {
	return APIPathTrack{
		Codec:      formatlabel.FormatToLabel(forma),
		CodecProps: formatToTrackCodecProps(forma),
	}
}

func gatherFormats(medias []*description.Media) []format.Format {
	n := 0
	for _, media := range medias {
		n += len(media.Formats)
	}

	if n == 0 {
		return nil
	}

	formats := make([]format.Format, n)
	n = 0

	for _, media := range medias {
		n += copy(formats[n:], media.Formats)
	}

	return formats
}

// FormatsToTracks returns tracks of given formats.
func FormatsToTracks(formats []format.Format) []APIPathTrack {
	ret := make([]APIPathTrack, len(formats))

	for i, forma := range formats {
		ret[i] = formatToTrack(forma)
	}

	return ret
}

// MediasToTracks returns tracks of given medias.
func MediasToTracks(medias []*description.Media) []APIPathTrack {
	return FormatsToTracks(gatherFormats(medias))
}
