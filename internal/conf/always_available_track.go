package conf

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// AlwaysAvailableTrack is an item of alwaysAvailableTracks.
type AlwaysAvailableTrack struct {
	Codec        AlwaysAvailableTrackCodec `json:"codec"`
	SampleRate   int                       `json:"sampleRate"`
	ChannelCount int                       `json:"channelCount"`
	MULaw        bool                      `json:"muLaw"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (t *AlwaysAvailableTrack) UnmarshalJSON(b []byte) error {
	type alias AlwaysAvailableTrack
	err := jsonwrapper.Unmarshal(b, (*alias)(t))
	if err != nil {
		return err
	}

	switch t.Codec {
	case CodecAV1, CodecVP9, CodecH265, CodecH264, CodecOpus:
		if t.SampleRate != 0 {
			return fmt.Errorf("sampleRate must not be specified for codec '%s'", t.Codec)
		}
		if t.ChannelCount != 0 {
			return fmt.Errorf("channelCount must not be specified for codec '%s'", t.Codec)
		}

	case CodecMPEG4Audio, CodecG711, CodecLPCM:
		if t.SampleRate == 0 {
			return fmt.Errorf("sampleRate is mandatory for codec '%s'", t.Codec)
		}
		if t.ChannelCount == 0 {
			return fmt.Errorf("channelCount is mandatory for codec '%s'", t.Codec)
		}

	default:
		return fmt.Errorf("unsupported codec '%s'", t.Codec)
	}

	return nil
}
