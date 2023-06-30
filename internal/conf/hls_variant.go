package conf

import (
	"encoding/json"
	"fmt"

	"github.com/bluenviron/gohlslib"
)

// HLSVariant is the hlsVariant parameter.
type HLSVariant gohlslib.MuxerVariant

// MarshalJSON implements json.Marshaler.
func (d HLSVariant) MarshalJSON() ([]byte, error) {
	var out string

	switch d {
	case HLSVariant(gohlslib.MuxerVariantMPEGTS):
		out = "mpegts"

	case HLSVariant(gohlslib.MuxerVariantFMP4):
		out = "fmp4"

	case HLSVariant(gohlslib.MuxerVariantLowLatency):
		out = "lowLatency"

	default:
		return nil, fmt.Errorf("invalid HLS variant: %v", d)
	}

	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *HLSVariant) UnmarshalJSON(b []byte) error {
	var in string
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}

	switch in {
	case "mpegts":
		*d = HLSVariant(gohlslib.MuxerVariantMPEGTS)

	case "fmp4":
		*d = HLSVariant(gohlslib.MuxerVariantFMP4)

	case "lowLatency":
		*d = HLSVariant(gohlslib.MuxerVariantLowLatency)

	default:
		return fmt.Errorf("invalid HLS variant: '%s'", in)
	}

	return nil
}

// UnmarshalEnv implements envUnmarshaler.
func (d *HLSVariant) UnmarshalEnv(s string) error {
	return d.UnmarshalJSON([]byte(`"` + s + `"`))
}
