package defs

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// Source is an entity that can provide a stream.
// it can be:
// - Publisher
// - staticsources.Handler
// - core.sourceRedirect
type Source interface {
	logger.Writer
	APISourceDescribe() APIPathSourceOrReader
}

// FormatsToCodecs returns the name of codecs of given formats.
func FormatsToCodecs(formats []format.Format) []string {
	ret := make([]string, len(formats))
	for i, forma := range formats {
		ret[i] = forma.Codec()
	}
	return ret
}

// FormatsInfo returns a description of formats.
func FormatsInfo(formats []format.Format) string {
	return fmt.Sprintf("%d %s (%s)",
		len(formats),
		func() string {
			if len(formats) == 1 {
				return "track"
			}
			return "tracks"
		}(),
		strings.Join(FormatsToCodecs(formats), ", "))
}

// MediasToCodecs returns the name of codecs of given formats.
func MediasToCodecs(medias []*description.Media) []string {
	var formats []format.Format
	for _, media := range medias {
		formats = append(formats, media.Formats...)
	}

	return FormatsToCodecs(formats)
}

// MediasInfo returns a description of medias.
func MediasInfo(medias []*description.Media) string {
	var formats []format.Format
	for _, media := range medias {
		formats = append(formats, media.Formats...)
	}

	return FormatsInfo(formats)
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

func fpsString(fps float64) string {
	// Round to 2 decimal places for display
	if fps == float64(int(fps)) {
		return fmt.Sprintf("%.0f", fps)
	}
	return fmt.Sprintf("%.2f", fps)
}

// FormatToTrack converts a single format to an APIPathTrack with detailed codec information.
func FormatToTrack(forma format.Format) APIPathTrack {
	track := APIPathTrack{
		Codec: forma.Codec(),
	}

	switch f := forma.(type) {
	case *format.H264:
		sps, _ := f.SafeParams()
		if len(sps) > 0 {
			var parsedSPS h264.SPS
			if err := parsedSPS.Unmarshal(sps); err == nil {
				width := parsedSPS.Width()
				height := parsedSPS.Height()
				track.Width = &width
				track.Height = &height

				fps := parsedSPS.FPS()
				if fps > 0 {
					fpsStr := fpsString(fps)
					track.FPS = &fpsStr
				}

				profile := h264ProfileString(parsedSPS.ProfileIdc)
				level := h264LevelString(parsedSPS.LevelIdc)
				track.H264Profile = &profile
				track.H264Level = &level
			}
		}

	case *format.H265:
		_, sps, _ := f.SafeParams()
		if len(sps) > 0 {
			var parsedSPS h265.SPS
			if err := parsedSPS.Unmarshal(sps); err == nil {
				width := parsedSPS.Width()
				height := parsedSPS.Height()
				track.Width = &width
				track.Height = &height

				fps := parsedSPS.FPS()
				if fps > 0 {
					fpsStr := fpsString(fps)
					track.FPS = &fpsStr
				}

				profile := h265ProfileString(parsedSPS.ProfileTierLevel.GeneralProfileIdc)
				level := h265LevelString(parsedSPS.ProfileTierLevel.GeneralLevelIdc)
				track.H265Profile = &profile
				track.H265Level = &level
			}
		}

	case *format.VP9:
		if f.ProfileID != nil {
			track.VP9Profile = f.ProfileID
		}

	case *format.AV1:
		if f.Profile != nil {
			track.AV1Profile = f.Profile
		}
		if f.LevelIdx != nil {
			track.AV1Level = f.LevelIdx
		}
		if f.Tier != nil {
			track.AV1Tier = f.Tier
		}

	case *format.Opus:
		channels := f.ChannelCount
		track.Channels = &channels
		sampleRate := f.ClockRate()
		track.SampleRate = &sampleRate

	case *format.MPEG4Audio:
		if f.Config != nil {
			channels := f.Config.ChannelCount
			track.Channels = &channels
			sampleRate := f.Config.SampleRate
			track.SampleRate = &sampleRate
		}

	case *format.G711:
		channels := f.ChannelCount
		if channels == 0 {
			channels = 1 // G.711 is typically mono
		}
		track.Channels = &channels
		sampleRate := f.SampleRate
		if sampleRate == 0 {
			sampleRate = 8000 // G.711 default sample rate
		}
		track.SampleRate = &sampleRate

	case *format.LPCM:
		bitDepth := f.BitDepth
		track.BitDepth = &bitDepth
		channels := f.ChannelCount
		track.Channels = &channels
		sampleRate := f.SampleRate
		track.SampleRate = &sampleRate
	}

	return track
}

// FormatsToTracks converts formats to detailed track information.
func FormatsToTracks(formats []format.Format) []APIPathTrack {
	ret := make([]APIPathTrack, len(formats))
	for i, forma := range formats {
		ret[i] = FormatToTrack(forma)
	}
	return ret
}

// MediasToTracks converts medias to detailed track information.
func MediasToTracks(medias []*description.Media) []APIPathTrack {
	var formats []format.Format
	for _, media := range medias {
		formats = append(formats, media.Formats...)
	}

	return FormatsToTracks(formats)
}
