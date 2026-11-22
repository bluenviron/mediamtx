package conf

import "fmt"

// AlwaysAvailableTrack is an item of alwaysAvailableTracks.
type AlwaysAvailableTrack struct {
	Codec        AlwaysAvailableTrackCodec `json:"codec"`
	SampleRate   int                       `json:"sampleRate"`
	ChannelCount int                       `json:"channelCount"`
	MULaw        bool                      `json:"muLaw"`
}

func (t AlwaysAvailableTrack) validate() error {
	switch t.Codec {
	case CodecAV1, CodecVP9, CodecH265, CodecH264, CodecOpus:
		if t.SampleRate != 0 {
			return fmt.Errorf("sampleRate must not be specified for codec '%s'", t.Codec)
		}
		if t.ChannelCount != 0 {
			return fmt.Errorf("channelCount must not be specified for codec '%s'", t.Codec)
		}

	case CodecMPEG4Audio:
		if t.SampleRate == 0 {
			return fmt.Errorf("sampleRate is mandatory for codec '%s'", t.Codec)
		}
		if t.SampleRate < 22050 {
			return fmt.Errorf("sampleRate must be greater than or equal to 22050 for codec '%s'", t.Codec)
		}
		if t.ChannelCount == 0 {
			return fmt.Errorf("channelCount is mandatory for codec '%s'", t.Codec)
		}

	case CodecG711, CodecLPCM:
		if t.SampleRate == 0 {
			return fmt.Errorf("sampleRate is mandatory for codec '%s'", t.Codec)
		}
		if t.SampleRate < 8000 {
			return fmt.Errorf("sampleRate must be greater than or equal to 8000 for codec '%s'", t.Codec)
		}
		if t.ChannelCount == 0 {
			return fmt.Errorf("channelCount is mandatory for codec '%s'", t.Codec)
		}

	default:
		return fmt.Errorf("unsupported codec '%s'", t.Codec)
	}

	return nil
}
