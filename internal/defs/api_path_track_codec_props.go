package defs

import (
	"fmt"
	"strconv"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	codecsh264 "github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	codecsh265 "github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
)

// APIPathTrackCodecProps contains codec-specific properties.
type APIPathTrackCodecProps interface {
	apiPathTrackCodecProps()
}

// APIPathTrackCodecPropsAV1 contains codec-specific properties of AV1.
type APIPathTrackCodecPropsAV1 struct {
	Width   int `json:"width"`
	Height  int `json:"height"`
	Profile int `json:"profile"`
	Level   int `json:"level"`
	Tier    int `json:"tier"`
}

func (*APIPathTrackCodecPropsAV1) apiPathTrackCodecProps() {}

// APIPathTrackCodecPropsVP9 contains codec-specific properties of VP9.
type APIPathTrackCodecPropsVP9 struct {
	Profile int `json:"profile"`
}

func (*APIPathTrackCodecPropsVP9) apiPathTrackCodecProps() {}

// APIPathTrackCodecPropsH265 contains codec-specific properties of H265.
type APIPathTrackCodecPropsH265 struct {
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Profile string `json:"profile"`
	Level   string `json:"level"`
}

func (*APIPathTrackCodecPropsH265) apiPathTrackCodecProps() {}

// APIPathTrackCodecPropsH264 contains codec-specific properties of H264.
type APIPathTrackCodecPropsH264 struct {
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Profile string `json:"profile"`
	Level   string `json:"level"`
}

func (*APIPathTrackCodecPropsH264) apiPathTrackCodecProps() {}

// APIPathTrackCodecPropsOpus contains codec-specific properties of Opus.
type APIPathTrackCodecPropsOpus struct {
	ChannelCount int `json:"channelCount"`
}

func (*APIPathTrackCodecPropsOpus) apiPathTrackCodecProps() {}

// APIPathTrackCodecPropsMPEG4Audio contains codec-specific properties of MPEG4Audio.
type APIPathTrackCodecPropsMPEG4Audio struct {
	SampleRate   int `json:"sampleRate"`
	ChannelCount int `json:"channelCount"`
}

func (*APIPathTrackCodecPropsMPEG4Audio) apiPathTrackCodecProps() {}

// APIPathTrackCodecPropsAC3 contains codec-specific properties of AC3.
type APIPathTrackCodecPropsAC3 struct {
	SampleRate   int `json:"sampleRate"`
	ChannelCount int `json:"channelCount"`
}

func (*APIPathTrackCodecPropsAC3) apiPathTrackCodecProps() {}

// APIPathTrackCodecPropsG711 contains codec-specific properties of G711.
type APIPathTrackCodecPropsG711 struct {
	MULaw        bool `json:"muLaw"`
	SampleRate   int  `json:"sampleRate"`
	ChannelCount int  `json:"channelCount"`
}

func (*APIPathTrackCodecPropsG711) apiPathTrackCodecProps() {}

// APIPathTrackCodecPropsLPCM contains codec-specific properties of LPCM.
type APIPathTrackCodecPropsLPCM struct {
	BitDepth     int `json:"bitDepth"`
	SampleRate   int `json:"sampleRate"`
	ChannelCount int `json:"channelCount"`
}

func (*APIPathTrackCodecPropsLPCM) apiPathTrackCodecProps() {}

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
