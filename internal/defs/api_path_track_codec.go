package defs

import (
	"fmt"
	"strconv"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	codecsh264 "github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	codecsh265 "github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
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

func formatToCodec(forma format.Format) APIPathTrackCodec {
	switch forma.(type) {
	// video
	case *format.AV1:
		return APIPathTrackCodecAV1
	case *format.VP9:
		return APIPathTrackCodecVP9
	case *format.VP8:
		return APIPathTrackCodecVP8
	case *format.H265:
		return APIPathTrackCodecH265
	case *format.H264:
		return APIPathTrackCodecH264
	case *format.MPEG4Video:
		return APIPathTrackCodecMPEG4Video
	case *format.MPEG1Video:
		return APIPathTrackCodecMPEG1Video
	case *format.MJPEG:
		return APIPathTrackCodecMJPEG
	// audio
	case *format.Opus:
		return APIPathTrackCodecOpus
	case *format.Vorbis:
		return APIPathTrackCodecVorbis
	case *format.MPEG4Audio:
		return APIPathTrackCodecMPEG4Audio
	case *format.MPEG4AudioLATM:
		return APIPathTrackCodecMPEG4AudioLATM
	case *format.MPEG1Audio:
		return APIPathTrackCodecMPEG1Audio
	case *format.AC3:
		return APIPathTrackCodecAC3
	case *format.Speex:
		return APIPathTrackCodecSpeex
	case *format.G726:
		return APIPathTrackCodecG726
	case *format.G722:
		return APIPathTrackCodecG722
	case *format.G711:
		return APIPathTrackCodecG711
	case *format.LPCM:
		return APIPathTrackCodecLPCM
	// other
	case *format.MPEGTS:
		return APIPathTrackCodecMPEGTS
	case *format.KLV:
		return APIPathTrackCodecKLV
	default:
		return APIPathTrackCodecGeneric
	}
}

func h264ProfileString(profileIdc uint8) string {
	switch profileIdc {
	case 66:
		return "Baseline"
	case 77:
		return "Main"
	case 88:
		return "Extended"
	case 100:
		return "High"
	case 110:
		return "High 10"
	case 122:
		return "High 4:2:2"
	case 244:
		return "High 4:4:4 Predictive"
	default:
		return strconv.Itoa(int(profileIdc))
	}
}

func h264LevelString(levelIdc uint8) string {
	major := levelIdc / 10
	minor := levelIdc % 10
	if minor == 0 {
		return fmt.Sprintf("%d", major)
	}
	return fmt.Sprintf("%d.%d", major, minor)
}

func h265ProfileString(profileIdc uint8) string {
	switch profileIdc {
	case 1:
		return "Main"
	case 2:
		return "Main 10"
	case 3:
		return "Main Still Picture"
	case 4:
		return "Range Extensions"
	case 5:
		return "High Throughput"
	case 6:
		return "Multiview Main"
	case 7:
		return "Scalable Main"
	case 8:
		return "3D Main"
	case 9:
		return "Screen Content Coding Extensions"
	case 10:
		return "Scalable Range Extensions"
	default:
		return strconv.Itoa(int(profileIdc))
	}
}

func h265LevelString(levelIdc uint8) string {
	// H.265 level_idc = 30 * level, so 3.0 = 90, 4.0 = 120, 4.1 = 123
	level := float64(levelIdc) / 30.0
	// Check if it's a clean level like 3.0, 4.0
	if levelIdc%30 == 0 {
		return fmt.Sprintf("%.0f", level)
	}
	return fmt.Sprintf("%.1f", level)
}

// FormatsToCodecs returns codecs of given formats.
func FormatsToCodecs(formats []format.Format) []APIPathTrackCodec {
	ret := make([]APIPathTrackCodec, len(formats))
	for i, forma := range formats {
		ret[i] = formatToCodec(forma)
	}
	return ret
}

func formatToTrack(forma format.Format) APIPathTrack {
	return APIPathTrack{
		Codec:      formatToCodec(forma),
		CodecProps: formatToTrackCodecProps(forma),
	}
}

func formatToTrackCodecProps(forma format.Format) APIPathTrackCodecProps {
	switch forma := forma.(type) {
	case *format.AV1:
		props := &APIPathTrackCodecPropsAV1{}

		if forma.Profile != nil {
			props.Profile = *forma.Profile
		}
		if forma.LevelIdx != nil {
			props.Level = *forma.LevelIdx
		}
		if forma.Tier != nil {
			props.Tier = *forma.Tier
		}

		return props

	case *format.VP9:
		props := &APIPathTrackCodecPropsVP9{}

		if forma.ProfileID != nil {
			props.Profile = *forma.ProfileID
		}

		return props

	case *format.H265:
		props := &APIPathTrackCodecPropsH265{}

		_, sps, _ := forma.SafeParams()
		if sps != nil {
			var s codecsh265.SPS
			if err := s.Unmarshal(sps); err == nil {
				props.Width = s.Width()
				props.Height = s.Height()
				props.Profile = h265ProfileString(s.ProfileTierLevel.GeneralProfileIdc)
				props.Level = h265LevelString(s.ProfileTierLevel.GeneralLevelIdc)
			}
		}

		return props

	case *format.H264:
		props := &APIPathTrackCodecPropsH264{}

		sps, _ := forma.SafeParams()
		if sps != nil {
			var s codecsh264.SPS
			if err := s.Unmarshal(sps); err == nil {
				props.Width = s.Width()
				props.Height = s.Height()
				props.Profile = h264ProfileString(s.ProfileIdc)
				props.Level = h264LevelString(s.LevelIdc)
			}
		}

		return props

	case *format.Opus:
		return &APIPathTrackCodecPropsOpus{
			ChannelCount: forma.ChannelCount,
		}

	case *format.MPEG4Audio:
		props := &APIPathTrackCodecPropsMPEG4Audio{}

		if forma.Config != nil {
			props.SampleRate = forma.Config.SampleRate
			props.ChannelCount = int(forma.Config.ChannelConfig)
		}

		return props

	case *format.AC3:
		return &APIPathTrackCodecPropsAC3{
			SampleRate:   forma.SampleRate,
			ChannelCount: forma.ChannelCount,
		}

	case *format.G711:
		return &APIPathTrackCodecPropsG711{
			MULaw:        forma.MULaw,
			SampleRate:   forma.SampleRate,
			ChannelCount: forma.ChannelCount,
		}

	case *format.LPCM:
		return &APIPathTrackCodecPropsLPCM{
			BitDepth:     forma.BitDepth,
			SampleRate:   forma.SampleRate,
			ChannelCount: forma.ChannelCount,
		}

	default:
		return nil
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

// MediasToCodecs returns codecs of given medias.
func MediasToCodecs(medias []*description.Media) []APIPathTrackCodec {
	return FormatsToCodecs(gatherFormats(medias))
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
