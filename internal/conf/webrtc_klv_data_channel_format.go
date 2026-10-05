package conf

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// WebRTCKLVDataChannelFormat is the webrtcKLVDataChannelFormat parameter.
type WebRTCKLVDataChannelFormat string

// supported values.
const (
	WebRTCKLVDataChannelFormatRaw   WebRTCKLVDataChannelFormat = "raw"
	WebRTCKLVDataChannelFormatTimed WebRTCKLVDataChannelFormat = "timed"
)

// UnmarshalJSON implements json.Unmarshaler.
func (d *WebRTCKLVDataChannelFormat) UnmarshalJSON(b []byte) error {
	type alias WebRTCKLVDataChannelFormat
	if err := jsonwrapper.Unmarshal(b, (*alias)(d)); err != nil {
		return err
	}

	switch *d {
	case WebRTCKLVDataChannelFormatRaw, WebRTCKLVDataChannelFormatTimed:

	default:
		return fmt.Errorf("invalid WebRTC KLV data channel format '%s'", *d)
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *WebRTCKLVDataChannelFormat) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}
