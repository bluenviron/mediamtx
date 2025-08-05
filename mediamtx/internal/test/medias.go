package test

import (
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
)

// MediaH264 is a dummy H264 media.
var MediaH264 = UniqueMediaH264()

// MediaMPEG4Audio is a dummy MPEG-4 audio media.
var MediaMPEG4Audio = UniqueMediaMPEG4Audio()

// UniqueMediaH264 is a dummy H264 media.
func UniqueMediaH264() *description.Media {
	return &description.Media{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{FormatH264},
	}
}

// UniqueMediaMPEG4Audio is a dummy MPEG-4 audio media.
func UniqueMediaMPEG4Audio() *description.Media {
	return &description.Media{
		Type:    description.MediaTypeAudio,
		Formats: []format.Format{FormatMPEG4Audio},
	}
}
