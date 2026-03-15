package defs

import (
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
)

// APIPathTrackCodec is a path track codec.
type APIPathTrackCodec string

// path track codecs.
const (
	// video
	APIPathTrackCodecAV1        APIPathTrackCodec = "AV1"
	APIPathTrackCodecVP9        APIPathTrackCodec = "VP9"
	APIPathTrackCodecVP8        APIPathTrackCodec = "VP8"
	APIPathTrackCodecH265       APIPathTrackCodec = "H265"
	APIPathTrackCodecH264       APIPathTrackCodec = "H264"
	APIPathTrackCodecMPEG4Video APIPathTrackCodec = "MPEG-4 Video"
	APIPathTrackCodecMPEG1Video APIPathTrackCodec = "MPEG-1 Video"
	APIPathTrackCodecMJPEG      APIPathTrackCodec = "MJPEG"
	// audio
	APIPathTrackCodecOpus           APIPathTrackCodec = "Opus"
	APIPathTrackCodecVorbis         APIPathTrackCodec = "Vorbis"
	APIPathTrackCodecMPEG4Audio     APIPathTrackCodec = "MPEG-4 Audio"
	APIPathTrackCodecMPEG4AudioLATM APIPathTrackCodec = "MPEG-4 Audio LATM"
	APIPathTrackCodecMPEG1Audio     APIPathTrackCodec = "MPEG-1 Audio"
	APIPathTrackCodecAC3            APIPathTrackCodec = "AC3"
	APIPathTrackCodecSpeex          APIPathTrackCodec = "Speex"
	APIPathTrackCodecG726           APIPathTrackCodec = "G726"
	APIPathTrackCodecG722           APIPathTrackCodec = "G722"
	APIPathTrackCodecG711           APIPathTrackCodec = "G711"
	APIPathTrackCodecLPCM           APIPathTrackCodec = "LPCM"
	// other
	APIPathTrackCodecMPEGTS  APIPathTrackCodec = "MPEG-TS"
	APIPathTrackCodecKLV     APIPathTrackCodec = "KLV"
	APIPathTrackCodecGeneric APIPathTrackCodec = "Generic"
)

// FormatsToCodecs returns codecs of given formats.
func FormatsToCodecs(formats []format.Format) []APIPathTrackCodec {
	ret := make([]APIPathTrackCodec, len(formats))

	for i, forma := range formats {
		switch forma.(type) {
		// video
		case *format.AV1:
			ret[i] = APIPathTrackCodecAV1
		case *format.VP9:
			ret[i] = APIPathTrackCodecVP9
		case *format.VP8:
			ret[i] = APIPathTrackCodecVP8
		case *format.H265:
			ret[i] = APIPathTrackCodecH265
		case *format.H264:
			ret[i] = APIPathTrackCodecH264
		case *format.MPEG4Video:
			ret[i] = APIPathTrackCodecMPEG4Video
		case *format.MPEG1Video:
			ret[i] = APIPathTrackCodecMPEG1Video
		case *format.MJPEG:
			ret[i] = APIPathTrackCodecMJPEG
			// audio
		case *format.Opus:
			ret[i] = APIPathTrackCodecOpus
		case *format.Vorbis:
			ret[i] = APIPathTrackCodecVorbis
		case *format.MPEG4Audio:
			ret[i] = APIPathTrackCodecMPEG4Audio
		case *format.MPEG4AudioLATM:
			ret[i] = APIPathTrackCodecMPEG4AudioLATM
		case *format.MPEG1Audio:
			ret[i] = APIPathTrackCodecMPEG1Audio
		case *format.AC3:
			ret[i] = APIPathTrackCodecAC3
		case *format.Speex:
			ret[i] = APIPathTrackCodecSpeex
		case *format.G726:
			ret[i] = APIPathTrackCodecG726
		case *format.G722:
			ret[i] = APIPathTrackCodecG722
		case *format.G711:
			ret[i] = APIPathTrackCodecG711
		case *format.LPCM:
			ret[i] = APIPathTrackCodecLPCM
			// other
		case *format.MPEGTS:
			ret[i] = APIPathTrackCodecMPEGTS
		case *format.KLV:
			ret[i] = APIPathTrackCodecKLV
		default:
			ret[i] = APIPathTrackCodecGeneric
		}
	}

	return ret
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

// MediasToCodecs returns codecs of given medias.
func MediasToCodecs(medias []*description.Media) []APIPathTrackCodec {
	return FormatsToCodecs(gatherFormats(medias))
}
