package conf

import (
	"encoding/json"
	"fmt"
)

// RtspRangeType is the type used in the Range header.
type RtspRangeType int

// supported rtsp range types.
const (
	RtspRangeTypeUndefined RtspRangeType = iota
	RtspRangeTypeClock
	RtspRangeTypeNPT
	RtspRangeTypeSMPTE
)

// MarshalJSON implements json.Marshaler.
func (d RtspRangeType) MarshalJSON() ([]byte, error) {
	var out string

	switch d {
	case RtspRangeTypeClock:
		out = "clock"

	case RtspRangeTypeNPT:
		out = "npt"

	case RtspRangeTypeSMPTE:
		out = "smpte"

	case RtspRangeTypeUndefined:
		out = ""

	default:
		return nil, fmt.Errorf("invalid rtsp range type: %v", d)
	}

	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *RtspRangeType) UnmarshalJSON(b []byte) error {
	var in string
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}

	switch in {
	case "clock":
		*d = RtspRangeTypeClock

	case "npt":
		*d = RtspRangeTypeNPT

	case "smpte":
		*d = RtspRangeTypeSMPTE

	case "":
		*d = RtspRangeTypeUndefined

	default:
		return fmt.Errorf("invalid rtsp range type: '%s'", in)
	}

	return nil
}

// UnmarshalEnv implements envUnmarshaler.
func (d *RtspRangeType) UnmarshalEnv(s string) error {
	return d.UnmarshalJSON([]byte(`"` + s + `"`))
}
