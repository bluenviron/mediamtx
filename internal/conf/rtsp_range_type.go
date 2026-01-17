package conf

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// RTSPRangeType is the type used in the Range header.
type RTSPRangeType string

// supported values.
const (
	RTSPRangeTypeUndefined RTSPRangeType = ""
	RTSPRangeTypeClock     RTSPRangeType = "clock"
	RTSPRangeTypeNPT       RTSPRangeType = "npt"
	RTSPRangeTypeSMPTE     RTSPRangeType = "smpte"
)

// UnmarshalJSON implements json.Unmarshaler.
func (d *RTSPRangeType) UnmarshalJSON(b []byte) error {
	type alias RTSPRangeType
	if err := jsonwrapper.Unmarshal(b, (*alias)(d)); err != nil {
		return err
	}

	switch *d {
	case RTSPRangeTypeUndefined, RTSPRangeTypeClock, RTSPRangeTypeNPT, RTSPRangeTypeSMPTE:

	default:
		return fmt.Errorf("invalid rtsp range type: '%s'", *d)
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *RTSPRangeType) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}
