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

func formatToTrack(outFormat format.Format) APIPathTrack {
	return APIPathTrack{
		Codec:      formatlabel.FormatToLabel(outFormat),
		CodecProps: formatToTrackCodecProps(outFormat),
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

func formatsToTracks(outFormats []format.Format) []APIPathTrack {
	ret := make([]APIPathTrack, len(outFormats))

	for i, outFormat := range outFormats {
		ret[i] = formatToTrack(outFormat)
	}

	return ret
}

// MediasToTracks returns tracks of given medias.
func MediasToTracks(outMedias []*description.Media) []APIPathTrack {
	return formatsToTracks(gatherFormats(outMedias))
}
