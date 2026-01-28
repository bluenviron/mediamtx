package conf

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// Codec is a codec of AlwaysAvailableTrack.
type Codec string

// available codecs.
const (
	CodecAV1        Codec = "AV1"
	CodecVP9        Codec = "VP9"
	CodecH265       Codec = "H265"
	CodecH264       Codec = "H264"
	CodecMPEG4Audio Codec = "MPEG4Audio"
	CodecOpus       Codec = "Opus"
	CodecG711       Codec = "G711"
	CodecLPCM       Codec = "LPCM"
)

// UnmarshalEnv implements env.Unmarshaler.
func (d *Codec) UnmarshalEnv(_ string, v string) error {
	return jsonwrapper.Unmarshal([]byte(`"`+v+`"`), d)
}

// AlwaysAvailableTrack is an item of alwaysAvailableTracks.
type AlwaysAvailableTrack struct {
	Codec        Codec `json:"codec"`
	SampleRate   int   `json:"sampleRate"`
	ChannelCount int   `json:"channelCount"`
	MULaw        bool  `json:"muLaw"`
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
