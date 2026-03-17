package conf

import "github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"

// AlwaysAvailableTrackCodec is a codec of AlwaysAvailableTrack.
type AlwaysAvailableTrackCodec string

// available codecs.
const (
	CodecAV1        AlwaysAvailableTrackCodec = "AV1"
	CodecVP9        AlwaysAvailableTrackCodec = "VP9"
	CodecH265       AlwaysAvailableTrackCodec = "H265"
	CodecH264       AlwaysAvailableTrackCodec = "H264"
	CodecMPEG4Audio AlwaysAvailableTrackCodec = "MPEG4Audio"
	CodecOpus       AlwaysAvailableTrackCodec = "Opus"
	CodecG711       AlwaysAvailableTrackCodec = "G711"
	CodecLPCM       AlwaysAvailableTrackCodec = "LPCM"
)

// UnmarshalEnv implements env.Unmarshaler.
func (d *AlwaysAvailableTrackCodec) UnmarshalEnv(_ string, v string) error {
	return jsonwrapper.Unmarshal([]byte(`"`+v+`"`), d)
}
