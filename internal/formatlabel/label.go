// Package formatlabel contains format label definitions.
package formatlabel

import (
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
)

// Label is a format label.
type Label string

// labels.
const (
	// video
	AV1        Label = "AV1"
	VP9        Label = "VP9"
	VP8        Label = "VP8"
	H265       Label = "H265"
	H264       Label = "H264"
	MPEG4Video Label = "MPEG-4 Video"
	MPEG1Video Label = "MPEG-1/2 Video"
	MJPEG      Label = "M-JPEG"
	// audio
	Opus           Label = "Opus"
	Vorbis         Label = "Vorbis"
	MPEG4Audio     Label = "MPEG-4 Audio"
	MPEG4AudioLATM Label = "MPEG-4 Audio LATM"
	MPEG1Audio     Label = "MPEG-1/2 Audio"
	AC3            Label = "AC3"
	Speex          Label = "Speex"
	G726           Label = "G726"
	G722           Label = "G722"
	G711           Label = "G711"
	LPCM           Label = "LPCM"
	// other
	MPEGTS  Label = "MPEG-TS"
	KLV     Label = "KLV"
	Generic Label = "Generic"
)

// FormatToLabel returns the label associated with a format.
func FormatToLabel(forma format.Format) Label {
	switch forma.(type) {
	// video
	case *format.AV1:
		return AV1
	case *format.VP9:
		return VP9
	case *format.VP8:
		return VP8
	case *format.H265:
		return H265
	case *format.H264:
		return H264
	case *format.MPEG4Video:
		return MPEG4Video
	case *format.MPEG1Video:
		return MPEG1Video
	case *format.MJPEG:
		return MJPEG
	// audio
	case *format.Opus:
		return Opus
	case *format.Vorbis:
		return Vorbis
	case *format.MPEG4Audio:
		return MPEG4Audio
	case *format.MPEG4AudioLATM:
		return MPEG4AudioLATM
	case *format.MPEG1Audio:
		return MPEG1Audio
	case *format.AC3:
		return AC3
	case *format.Speex:
		return Speex
	case *format.G726:
		return G726
	case *format.G722:
		return G722
	case *format.G711:
		return G711
	case *format.LPCM:
		return LPCM
	// other
	case *format.MPEGTS:
		return MPEGTS
	case *format.KLV:
		return KLV
	default:
		return Generic
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

// FormatsToLabels returns labels of given formats.
func FormatsToLabels(formats []format.Format) []Label {
	ret := make([]Label, len(formats))
	for i, forma := range formats {
		ret[i] = FormatToLabel(forma)
	}
	return ret
}

// MediasToLabels returns labels of given medias.
func MediasToLabels(medias []*description.Media) []Label {
	return FormatsToLabels(gatherFormats(medias))
}
